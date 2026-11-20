package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// eipMetrics 弹性公网IP 监控：阿里云走 CMS acs_vpc_eip。
func (s *MetricsService) eipMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunEipMetrics(ds, data, lookback, step, res)
	case model.DataSourceTencent:
		res.Message = "腾讯云 EIP 监控对接中"
		return res, nil
	default:
		res.Message = "该来源暂不支持 EIP 监控（" + ds.Type + "）"
		return res, nil
	}
}

// aliyunEipMetrics 阿里云 EIP：进出带宽、带宽使用率、包速率、丢包。
// 关联键：resource_id（eip-xxx）。
func (s *MetricsService) aliyunEipMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "eip-") {
		res.Message = "该 EIP 无阿里云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	region := firstField(data, "region", "bk_zone", "bk_region")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := fmt.Sprintf("%d", step)

	metrics := []struct {
		key, name, unit, label, metric string
	}{
		{"rx", "入带宽", "bps", "入", "net_rx.rate"},
		{"tx", "出带宽", "bps", "出", "net_tx.rate"},
		{"rxpps", "入包速率", "pps", "入", "net_rxPkgs.rate"},
		{"txpps", "出包速率", "pps", "出", "net_txPkgs.rate"},
		{"droptx", "限速丢包", "pps", "出", "out_ratelimit_drop_speed"},
	}
	var rxPts, txPts [][2]float64
	for _, m := range metrics {
		groups, err := cmsMetric(ds, endpoint, "acs_vpc_eip", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: m.label, Points: groups[0].points})
		if m.key == "rx" {
			rxPts = groups[0].points
		} else if m.key == "tx" {
			txPts = groups[0].points
		}
	}

	// 带宽使用率：实时带宽(bit/s) ÷ 峰值带宽(bandwidth 字段, Mbps) ×100，比 CMS 的百分比指标可靠
	capBps := firstFloat(data, "bandwidth", "band_width", "max_bandwidth") * 1e6
	if capBps > 0 {
		if p := ratioPercent(rxPts, capBps); len(p) > 0 {
			res.Series = append(res.Series, MetricSeries{Key: "in_pct", Name: "入向使用率", Unit: "%", Label: "入", Points: p})
		}
		if p := ratioPercent(txPts, capBps); len(p) > 0 {
			res.Series = append(res.Series, MetricSeries{Key: "out_pct", Name: "出向使用率", Unit: "%", Label: "出", Points: p})
		}
	}

	res.Cards = []MetricCard{
		{Key: "band", Name: "带宽（进/出）", Unit: "bps", SeriesKeys: []string{"rx", "tx"}},
		{Key: "band_pct", Name: "带宽使用率", Unit: "%", SeriesKeys: []string{"in_pct", "out_pct"}},
		{Key: "pps", Name: "包速率", Unit: "pps", SeriesKeys: []string{"rxpps", "txpps"}},
		{Key: "drop", Name: "限速丢包", Unit: "pps", SeriesKeys: []string{"droptx"}},
	}
	res.DetailURL = fmt.Sprintf("https://vpc.console.aliyun.com/eip/%s/eips/%s", region, instID)

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 EIP 在云监控中暂无数据"
	}
	return res, nil
}

// firstFloat 取 data 中首个可转数值的字段。
func firstFloat(data map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := data[k]; ok {
			switch t := v.(type) {
			case float64:
				return t
			case int:
				return float64(t)
			case string:
				if f, ok := toFloat(t); ok {
					return f
				}
			}
		}
	}
	return 0
}

// ratioPercent 把速率点集按 cap 换算成百分比 [ts, val/cap*100]。
func ratioPercent(pts [][2]float64, cap float64) [][2]float64 {
	if cap <= 0 || len(pts) == 0 {
		return nil
	}
	out := make([][2]float64, 0, len(pts))
	for _, p := range pts {
		out = append(out, [2]float64{p[0], p[1] / cap * 100})
	}
	return out
}
