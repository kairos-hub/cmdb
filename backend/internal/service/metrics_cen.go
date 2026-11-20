package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// cenMetrics 云企业网(CEN) 监控：阿里云走 CMS acs_cen。
func (s *MetricsService) cenMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunCenMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持云企业网监控（" + ds.Type + "）"
		return res, nil
	}
}

// cenCmsAgg 查询 acs_cen 指标（维度仅给 cenId，返回各 Attachment/地域对的数据点），
// 按时间戳跨维度聚合为 CEN 级单条序列：速率/流量 sum，利用率类 max。
func cenCmsAgg(ds *model.DataSource, endpoint, metric, cenID string, startMs, endMs int64, period string, useMax bool) [][2]float64 {
	extra := map[string]string{
		"Namespace":  "acs_cen",
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
		"Dimensions": fmt.Sprintf(`{"cenId":"%s"}`, cenID),
	}
	body, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-01-01", "DescribeMetricList", extra)
	if err != nil {
		return nil
	}
	var out struct {
		Datapoints string `json:"Datapoints"`
	}
	if json.Unmarshal(body, &out) != nil || strings.TrimSpace(out.Datapoints) == "" {
		return nil
	}
	var dps []map[string]interface{}
	if json.Unmarshal([]byte(out.Datapoints), &dps) != nil {
		return nil
	}
	agg := map[float64]float64{}
	for _, dp := range dps {
		ts, ok := cmsNum(dp["timestamp"])
		if !ok {
			continue
		}
		val, ok := cmsDatapointVal(dp)
		if !ok {
			continue
		}
		t := ts / 1000.0 // ms→s
		if useMax {
			if cur, had := agg[t]; !had || val > cur {
				agg[t] = val
			}
		} else {
			agg[t] += val
		}
	}
	pts := make([][2]float64, 0, len(agg))
	for t, v := range agg {
		pts = append(pts, [2]float64{t, v})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
	return pts
}

// aliyunCenMetrics 阿里云 CEN：CMS acs_cen。关联键=resource_id(cen-xxx)。
// CEN 监控以 Attachment(连接) 与地域间带宽为维度，这里按 cenId 聚合为实例级序列。
// 实例无跨地域带宽/连接流量时不产生监控数据，返回「暂无数据」。
func (s *MetricsService) aliyunCenMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "cen-") {
		res.Message = "该云企业网无阿里云实例 ID（resource_id），无法关联云监控"
		return res, nil
	}
	region := firstField(data, "region", "bk_region", "bk_zone")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Target = instID
	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := fmt.Sprintf("%d", step)

	add := func(key, name, unit, label, metric string, useMax bool) {
		pts := cenCmsAgg(ds, endpoint, metric, instID, startMs, endMs, period, useMax)
		if len(pts) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
	}
	// 经典边界路由器(VBR)：按 cenId 聚合各 VBR 的流入/流出带宽、健康检查延时/丢包
	add("in_rate", "边界路由器流入带宽", "bps", "流入", "VBRInternetInRate", false)
	add("out_rate", "边界路由器流出带宽", "bps", "流出", "VBRInternetOutRate", false)
	add("vbr_lat", "健康检查延时", "ms", "", "VBRHealthyCheckLatency", true)
	add("vbr_loss", "健康检查丢包率", "%", "", "VBRHealthyCheckLossRate", true)
	// 企业版转发路由器(TR)/地域间带宽：无则自动隐藏（兼容两种 CEN 架构）
	add("tr_in", "连接流入带宽", "bps", "流入", "AttachmentInRate", false)
	add("tr_out", "连接流出带宽", "bps", "流出", "AttachmentOutRate", false)
	add("region_rate", "地域间带宽速率", "bps", "", "InterRegionAverageBandwidthRate", false)
	add("region_util", "地域间带宽利用率", "%", "", "InterRegionAverageBandwidthUtilization", true)

	res.Cards = []MetricCard{
		{Key: "flow", Name: "边界路由器带宽（流入/流出）", Unit: "bps", SeriesKeys: []string{"in_rate", "out_rate"}},
		{Key: "vbr_lat", Name: "健康检查延时", Unit: "ms", SeriesKeys: []string{"vbr_lat"}},
		{Key: "vbr_loss", Name: "健康检查丢包率", Unit: "%", SeriesKeys: []string{"vbr_loss"}},
		{Key: "tr_flow", Name: "连接带宽（流入/流出）", Unit: "bps", SeriesKeys: []string{"tr_in", "tr_out"}},
		{Key: "region_rate", Name: "地域间带宽速率", Unit: "bps", SeriesKeys: []string{"region_rate"}},
		{Key: "region_util", Name: "地域间带宽利用率", Unit: "%", SeriesKeys: []string{"region_util"}},
	}
	// 云监控边界路由器(VBR)监控页：dimension=cenId:<cenId>（%3A 即冒号）
	res.DetailURL = fmt.Sprintf("https://cloudmonitornext.console.aliyun.com/productMonitorChart?category=cen_vbr&dimension=cenId%%3A%s&region=all", instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该云企业网在云监控中暂无数据（无跨地域带宽/连接流量时不产生监控数据）"
	}
	return res, nil
}
