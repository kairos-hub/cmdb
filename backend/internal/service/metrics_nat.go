package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// natgwMetrics NAT 网关监控：阿里云走 CMS acs_nat_gateway（实例级，维度 instanceId）。
func (s *MetricsService) natgwMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourceAliyun {
		res.Message = "该来源暂不支持 NAT 网关监控（" + ds.Type + "）"
		return res, nil
	}
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "ngw-") {
		res.Message = "该 NAT 网关无阿里云实例 ID（resource_id），无法关联云监控"
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

	add := func(key, name, unit, label, metric string) {
		groups, err := cmsMetric(ds, endpoint, "acs_nat_gateway", instID, metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 || len(groups[0].points) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: groups[0].points})
	}
	add("active", "并发连接数", "count", "", "SessionActiveConnection")
	add("new", "新建连接速率", "/s", "", "SessionNewConnection")
	add("water", "并发连接水位", "%", "", "SessionActiveConnectionWaterLever")
	add("bw_out", "出公网带宽", "Bps", "出公网", "BWRateOutToOutside")
	add("bw_in", "入公网带宽", "Bps", "入公网", "BWRateInFromOutside")
	add("pps_out", "出公网包速率", "pps", "出公网", "PPSRateOutToOutside")
	add("pps_in", "入公网包速率", "pps", "入公网", "PPSRateInFromOutside")
	add("drop_conn", "并发超规格丢连接", "/s", "丢连接", "SessionLimitDropConnection")
	add("port_err", "端口分配失败", "/s", "端口失败", "ErrorPortAllocationRate")

	res.Cards = []MetricCard{
		{Key: "active", Name: "并发连接数", Unit: "count", SeriesKeys: []string{"active"}},
		{Key: "new", Name: "新建连接速率", Unit: "/s", SeriesKeys: []string{"new"}},
		{Key: "water", Name: "并发连接水位", Unit: "%", SeriesKeys: []string{"water"}},
		{Key: "bw", Name: "公网带宽（出/入）", Unit: "Bps", SeriesKeys: []string{"bw_out", "bw_in"}},
		{Key: "pps", Name: "公网包速率（出/入）", Unit: "pps", SeriesKeys: []string{"pps_out", "pps_in"}},
		{Key: "drop", Name: "异常（丢连接/端口失败）", Unit: "/s", SeriesKeys: []string{"drop_conn", "port_err"}},
	}
	res.DetailURL = fmt.Sprintf("https://vpc.console.aliyun.com/nat/%s/nats/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 NAT 网关在云监控中暂无数据"
	}
	return res, nil
}
