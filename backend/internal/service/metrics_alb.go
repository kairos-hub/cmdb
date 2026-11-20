package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// albMetrics ALB(应用型负载均衡) 监控：阿里云走 CMS acs_alb。
func (s *MetricsService) albMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourceAliyun:
		return s.aliyunAlbMetrics(ds, data, lookback, step, res)
	default:
		res.Message = "该来源暂不支持 ALB 监控（" + ds.Type + "）"
		return res, nil
	}
}

// aliyunAlbMetrics 阿里云 ALB：CMS acs_alb。关联键=resource_id(alb-xxx)。空闲实例流量类指标可能无数据点。
func (s *MetricsService) aliyunAlbMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "alb-") {
		res.Message = "该 ALB 无阿里云实例 ID（resource_id），无法关联云监控"
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
		{"in", "入带宽", "bps", "入", "LoadBalancerInBits"},
		{"out", "出带宽", "bps", "出", "LoadBalancerOutBits"},
		{"conn_active", "活跃连接", "", "活跃", "LoadBalancerActiveConnection"},
		{"conn_new", "新建连接", "/s", "新建", "LoadBalancerNewConnection"},
		{"qps", "QPS", "/s", "", "LoadBalancerQPS"},
		{"code2xx", "2xx", "/s", "2xx", "LoadBalancerHTTPCode2XX"},
		{"code4xx", "4xx", "/s", "4xx", "LoadBalancerHTTPCode4XX"},
		{"code5xx", "5xx", "/s", "5xx", "LoadBalancerHTTPCode5XX"},
		{"healthy", "健康后端", "", "", "LoadBalancerHealthyHostCount"},
		{"lcu", "LCU 用量", "", "", "ConsumedLCUs"},
	}
	for _, m := range metrics {
		groups, err := cmsMetric(ds, endpoint, "acs_alb", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: m.label, Points: groups[0].points})
	}

	res.Cards = []MetricCard{
		{Key: "flow", Name: "带宽（入/出）", Unit: "bps", SeriesKeys: []string{"in", "out"}},
		{Key: "conn", Name: "连接数（活跃/新建）", Unit: "", SeriesKeys: []string{"conn_active", "conn_new"}},
		{Key: "qps", Name: "QPS", Unit: "/s", SeriesKeys: []string{"qps"}},
		{Key: "code", Name: "HTTP 状态码", Unit: "/s", SeriesKeys: []string{"code2xx", "code4xx", "code5xx"}},
		{Key: "healthy", Name: "健康后端", Unit: "", SeriesKeys: []string{"healthy"}},
		{Key: "lcu", Name: "LCU 用量", Unit: "", SeriesKeys: []string{"lcu"}},
	}
	res.DetailURL = fmt.Sprintf("https://slb.console.aliyun.com/alb/%s/albs/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 ALB 在云监控中暂无数据"
	}
	return res, nil
}
