package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// apigMetrics 云原生API网关(APIG)监控：阿里云走 CMS acs_cnapigateway（实例级，维度 instanceId）。
// 覆盖网关实例的黄金指标（QPS/响应时间/成功率/状态码）与资源水位（CPU/内存/连接/网络）。
func (s *MetricsService) apigMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourceAliyun {
		res.Message = "该来源暂不支持 API 网关监控（" + ds.Type + "）"
		return res, nil
	}
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "gw-") {
		res.Message = "该 API 网关无阿里云云原生网关实例 ID（resource_id），无法关联云监控"
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

	// 实例级单线指标（维度仅 instanceId，返回单条序列）
	add := func(key, name, unit, label, metric string) {
		groups, err := cmsMetric(ds, endpoint, "acs_cnapigateway", instID, metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 || len(groups[0].points) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: groups[0].points})
	}
	// 按状态码的指标为「按路由」QPS（次/秒）维度：跨路由按时间戳求和得实例级总量，
	// 再乘以采样周期(step 秒)换算成每个时间桶的「请求数(次)」，与阿里云看板「请求数」口径一致。
	addCount := func(key, name, metric string) {
		pts := apigSumByTS(ds, endpoint, instID, metric, startMs, endMs, period)
		if len(pts) == 0 {
			return
		}
		for i := range pts {
			pts[i][1] = pts[i][1] * float64(step)
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: "count", Points: pts})
	}

	add("qps", "请求 QPS", "/s", "", "EnvoyDownstreamQps")
	add("rt_down", "网关总延迟", "ms", "网关", "EnvoyDownstreamRtAvg")
	add("rt_up", "后端延迟", "ms", "后端", "EnvoyUpstreamRtAvg")
	add("succ_down", "网关成功率", "%", "网关", "EnvoyDownstreamSuccessRate")
	add("succ_up", "后端成功率", "%", "后端", "EnvoyUpstreamSuccessRate")
	addCount("code_2xx", "2XX 请求数", "EnvoyRouteQps2XX")
	addCount("code_4xx", "4XX 请求数", "EnvoyRouteQps4XX")
	addCount("code_5xx", "5XX 请求数", "EnvoyRouteQps5XX")
	add("cpu", "CPU 使用率", "%", "", "EnvoyCpuUsageRate")
	add("mem", "内存使用率", "%", "", "EnvoyMemoryUsageRate")
	add("conn_active", "活跃连接数", "count", "活跃连接", "EnvoyClientActiveConnection")
	add("conn_new", "新建连接速率", "/s", "新建连接", "EnvoyClientNewConnection")
	// 公网带宽：IPv4 EIP + IPv6 公网网关（单位 bit/s，与阿里云看板口径一致）。
	add("pub_in4", "公网入(IPv4)", "bps", "IPv4 入", "Ipv4EipRateIn")
	add("pub_out4", "公网出(IPv4)", "bps", "IPv4 出", "Ipv4EipRateOut")
	add("pub_in6", "公网入(IPv6)", "bps", "IPv6 入", "Ipv6GatewayRateIn")
	add("pub_out6", "公网出(IPv6)", "bps", "IPv6 出", "Ipv6GatewayRateOut")

	res.Cards = []MetricCard{
		{Key: "qps", Name: "请求 QPS", Unit: "/s", SeriesKeys: []string{"qps"}},
		{Key: "rt", Name: "平均响应时间（网关/后端）", Unit: "ms", SeriesKeys: []string{"rt_down", "rt_up"}},
		{Key: "succ", Name: "请求成功率（网关/后端）", Unit: "%", SeriesKeys: []string{"succ_down", "succ_up"}},
		{Key: "code2", Name: "2XX 请求数", Unit: "count", SeriesKeys: []string{"code_2xx"}},
		{Key: "code4", Name: "4XX 请求数", Unit: "count", SeriesKeys: []string{"code_4xx"}},
		{Key: "code5", Name: "5XX 请求数", Unit: "count", SeriesKeys: []string{"code_5xx"}},
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "conn", Name: "客户端连接（活跃/新建）", Unit: "count", SeriesKeys: []string{"conn_active", "conn_new"}},
		{Key: "pubbw", Name: "公网带宽（入/出）", Unit: "bps", SeriesKeys: []string{"pub_in4", "pub_out4", "pub_in6", "pub_out6"}},
	}
	edition := firstField(data, "edition")
	if edition == "" {
		edition = "Professional"
	}
	res.DetailURL = fmt.Sprintf(
		"https://apig.console.aliyun.com/instance/%s?region=%s#/%s/gateway/%s/observe/monitor?region=%s&gatewayEdition=%s",
		instID, region, region, instID, region, edition)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 API 网关在云监控中暂无数据"
	}
	return res, nil
}

// apigSumByTS 拉取「按路由」维度的指标，按时间戳跨路由求和，得到实例级总量序列。
func apigSumByTS(ds *model.DataSource, endpoint, instID, metric string, startMs, endMs int64, period string) [][2]float64 {
	groups, err := cmsMetric(ds, endpoint, "acs_cnapigateway", instID, metric, startMs, endMs, period, "route")
	if err != nil || len(groups) == 0 {
		return nil
	}
	sum := map[float64]float64{}
	for _, g := range groups {
		for _, p := range g.points {
			sum[p[0]] += p[1]
		}
	}
	out := make([][2]float64, 0, len(sum))
	for ts, v := range sum {
		out = append(out, [2]float64{ts, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
