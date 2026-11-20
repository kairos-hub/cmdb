package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// nlbMetrics NLB(网络型负载均衡) 监控：阿里云走 CMS acs_nlb。
func (s *MetricsService) nlbMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunNlbMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持 NLB 监控（" + ds.Type + "）"
		return res, nil
	}
}

// aliyunNlbMetrics 阿里云 NLB：CMS acs_nlb。关联键=resource_id(nlb-xxx)。
func (s *MetricsService) aliyunNlbMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "nlb-") {
		res.Message = "该 NLB 无阿里云实例 ID（resource_id），无法关联云监控"
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
		{"prx", "入包速率", "pps", "入", "InstancePacketRX"},
		{"ptx", "出包速率", "pps", "出", "InstancePacketTX"},
		{"healthy", "健康后端", "", "健康", "NlbInstanceHeathyServerCount"},
		{"unhealthy", "异常后端", "", "异常", "InstanceUnhealthyServerCount"},
		{"drop", "丢弃连接", "/s", "", "InstanceDropConnection"},
		{"lcu", "LCU 用量", "", "", "ConsumedLCUs"},
	}
	for _, m := range metrics {
		groups, err := cmsMetric(ds, endpoint, "acs_nlb", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: m.label, Points: groups[0].points})
	}

	res.Cards = []MetricCard{
		{Key: "flow", Name: "带宽（流入/流出）", Unit: "bps", SeriesKeys: []string{"rx", "tx"}},
		{Key: "conn", Name: "连接数（活跃/新建）", Unit: "", SeriesKeys: []string{"conn_active", "conn_new"}},
		{Key: "pps", Name: "包速率（入/出）", Unit: "pps", SeriesKeys: []string{"prx", "ptx"}},
		{Key: "health", Name: "后端服务器（健康/异常）", Unit: "", SeriesKeys: []string{"healthy", "unhealthy"}},
		{Key: "drop", Name: "丢弃连接", Unit: "/s", SeriesKeys: []string{"drop"}},
		{Key: "lcu", Name: "LCU 用量", Unit: "", SeriesKeys: []string{"lcu"}},
	}
	res.DetailURL = fmt.Sprintf("https://slb.console.aliyun.com/nlb/%s/nlbs/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 NLB 在云监控中暂无数据"
	}
	return res, nil
}
