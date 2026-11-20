package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// clbMetrics CLB(传统型负载均衡 SLB) 监控：阿里云走 CMS acs_slb_dashboard。
func (s *MetricsService) clbMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunClbMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持 CLB 监控（" + ds.Type + "）"
		return res, nil
	}
}

// aliyunClbMetrics 阿里云 CLB：CMS acs_slb_dashboard。关联键=resource_id(lb-xxx)。
// 七层指标(QPS/状态码)仅七层监听有数据，四层实例自动剪卡。
func (s *MetricsService) aliyunClbMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "lb-") {
		res.Message = "该 CLB 无阿里云实例 ID（resource_id），无法关联云监控"
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
		{"rx", "流入带宽", "bps", "流入", "InstanceTrafficRX"},
		{"tx", "流出带宽", "bps", "流出", "InstanceTrafficTX"},
		{"conn_active", "活跃连接", "", "活跃", "InstanceActiveConnection"},
		{"conn_new", "新建连接", "", "新建", "InstanceNewConnection"},
		{"drop", "丢弃连接", "/s", "", "InstanceDropConnection"},
		{"qps", "QPS", "/s", "", "InstanceQps"},
		{"code5xx", "5xx/s", "/s", "", "InstanceStatusCode5xx"},
	}
	var rxPts, txPts [][2]float64
	for _, m := range metrics {
		groups, err := cmsMetric(ds, endpoint, "acs_slb_dashboard", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: m.label, Points: groups[0].points})
		switch m.key {
		case "rx":
			rxPts = groups[0].points
		case "tx":
			txPts = groups[0].points
		}
	}

	// 带宽使用率：流量(bit/s) ÷ 购买带宽(bandwidth 字段, Mbps)；所有 CLB 都能算，与带宽卡成对
	capBps := firstFloat(data, "bandwidth") * 1e6
	if capBps > 0 {
		if p := ratioPercent(rxPts, capBps); len(p) > 0 {
			res.Series = append(res.Series, MetricSeries{Key: "in_pct", Name: "流入使用率", Unit: "%", Label: "流入", Points: p})
		}
		if p := ratioPercent(txPts, capBps); len(p) > 0 {
			res.Series = append(res.Series, MetricSeries{Key: "out_pct", Name: "流出使用率", Unit: "%", Label: "流出", Points: p})
		}
	}

	res.Cards = []MetricCard{
		{Key: "flow", Name: "带宽（流入/流出）", Unit: "bps", SeriesKeys: []string{"rx", "tx"}},
		{Key: "flow_pct", Name: "带宽使用率", Unit: "%", SeriesKeys: []string{"in_pct", "out_pct"}},
		{Key: "conn", Name: "连接数（活跃/新建）", Unit: "", SeriesKeys: []string{"conn_active", "conn_new"}},
		{Key: "drop", Name: "丢弃连接", Unit: "/s", SeriesKeys: []string{"drop"}},
		{Key: "qps", Name: "QPS（七层）", Unit: "/s", SeriesKeys: []string{"qps"}},
		{Key: "code5xx", Name: "5xx（七层）", Unit: "/s", SeriesKeys: []string{"code5xx"}},
	}
	res.DetailURL = fmt.Sprintf("https://slb.console.aliyun.com/slb/%s/slbs/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 CLB 在云监控中暂无数据"
	}
	return res, nil
}
