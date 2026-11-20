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

// aliyunRedisClass 实时查 Redis 实例规格（控制台监控页深链需要），失败返回空。
func aliyunRedisClass(ds *model.DataSource, region, instID string) string {
	body, err := aliRPC(redisEndpoint(region), ds.Username, ds.Password, "2015-01-01", "DescribeInstances",
		map[string]string{"InstanceIds": instID})
	if err != nil {
		return ""
	}
	var out struct {
		Instances struct {
			KVStoreInstance []struct {
				InstanceClass string `json:"InstanceClass"`
			} `json:"KVStoreInstance"`
		} `json:"Instances"`
	}
	if json.Unmarshal(body, &out) != nil || len(out.Instances.KVStoreInstance) == 0 {
		return ""
	}
	return out.Instances.KVStoreInstance[0].InstanceClass
}

// cmsKvstoreSeries 取阿里云 Redis(acs_kvstore) 指标：不带维度拉取全量，按 instId 前缀过滤本实例各分片，
// 再按时间戳聚合（agg: sum/avg/max）。返回 [ts秒,val] 点集。
func cmsKvstoreSeries(ds *model.DataSource, endpoint, instID, metric string, startMs, endMs int64, period, agg string) [][2]float64 {
	extra := map[string]string{
		"Namespace":  "acs_kvstore",
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
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
	// 按时间戳聚合本实例各分片
	type acc struct {
		sum float64
		max float64
		n   int
	}
	byTs := map[int64]*acc{}
	for _, dp := range dps {
		id := fmt.Sprint(dp["instanceId"])
		if id != instID && !strings.HasPrefix(id, instID+"-") {
			continue
		}
		ts, ok := cmsNum(dp["timestamp"])
		if !ok {
			continue
		}
		val, ok := cmsDatapointVal(dp)
		if !ok {
			continue
		}
		k := int64(ts)
		a := byTs[k]
		if a == nil {
			a = &acc{}
			byTs[k] = a
		}
		a.sum += val
		if a.n == 0 || val > a.max {
			a.max = val
		}
		a.n++
	}
	pts := make([][2]float64, 0, len(byTs))
	for ts, a := range byTs {
		v := a.sum
		switch agg {
		case "avg":
			v = a.sum / float64(a.n)
		case "max":
			v = a.max
		}
		pts = append(pts, [2]float64{float64(ts) / 1000.0, v})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
	return pts
}

// redisMetrics Redis 实例监控：自建→redis_exporter(+node_exporter 补 CPU/内存)，阿里云→CMS acs_kvstore。
func (s *MetricsService) redisMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourcePrometheus:
		return s.promRedisMetrics(ds, data, lookback, step, res)
	case model.DataSourceAliyun:
		return s.aliyunRedisMetrics(ds, data, lookback, step, res)
	case model.DataSourceTencent:
		res.Message = "腾讯云 Redis 监控对接中"
		return res, nil
	default:
		res.Message = "该来源暂不支持 Redis 监控（" + ds.Type + "）"
		return res, nil
	}
}

// promRedisMetrics 自建 Redis：redis_exporter + 同机 node_exporter。关联键=endpoint 的 IP。
func (s *MetricsService) promRedisMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	ip := firstField(data, "endpoint", "bk_host_innerip", "ip")
	if i := strings.IndexAny(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	if ip == "" {
		res.Message = "该 Redis 无 endpoint/IP，无法关联监控"
		return res, nil
	}
	res.Target = ip
	sel := fmt.Sprintf("instance=~`%s(:[0-9]+)?`", regexEscape(ip))

	end := time.Now().Unix()
	start := end - int64(lookback)
	client := noProxyClient(12 * time.Second)
	base := strings.TrimRight(ds.URL, "/")
	add := func(key, name, unit, label, expr string) {
		if pts, err := promQueryRange(client, base, ds, expr, start, end, step); err == nil && len(pts) > 0 {
			res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
		}
	}
	// 宿主机 CPU/内存（redis_exporter 不含，取同机 node_exporter）
	add("cpu", "CPU 使用率", "%", "", fmt.Sprintf(`100 - (avg(rate(node_cpu_seconds_total{%s,mode="idle"}[5m])) * 100)`, sel))
	add("mem", "内存使用率", "%", "", fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{%s} / node_memory_MemTotal_bytes{%s}) * 100`, sel, sel))
	// Redis 自身指标
	add("conn", "连接数", "", "", fmt.Sprintf(`redis_connected_clients{%s}`, sel))
	add("qps", "QPS", "/s", "", fmt.Sprintf(`sum(rate(redis_commands_total{%s}[5m]))`, sel))
	add("hit", "命中率", "%", "", fmt.Sprintf(`rate(redis_keyspace_hits_total{%s}[5m]) / (rate(redis_keyspace_hits_total{%s}[5m]) + rate(redis_keyspace_misses_total{%s}[5m]) + 0.0000001) * 100`, sel, sel, sel))
	add("rmem", "Redis 内存", "bytes", "", fmt.Sprintf(`redis_memory_used_bytes{%s}`, sel))
	add("keys", "键数量", "", "", fmt.Sprintf(`sum(redis_db_keys{%s})`, sel))
	add("net_in", "网络入", "bps", "入", fmt.Sprintf(`rate(redis_net_input_bytes_total{%s}[5m]) * 8`, sel))
	add("net_out", "网络出", "bps", "出", fmt.Sprintf(`rate(redis_net_output_bytes_total{%s}[5m]) * 8`, sel))

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "conn", Name: "连接数", Unit: "", SeriesKeys: []string{"conn"}},
		{Key: "qps", Name: "QPS", Unit: "/s", SeriesKeys: []string{"qps"}},
		{Key: "hit", Name: "命中率", Unit: "%", SeriesKeys: []string{"hit"}},
		{Key: "rmem", Name: "Redis 内存", Unit: "bytes", SeriesKeys: []string{"rmem"}},
		{Key: "keys", Name: "键数量", Unit: "", SeriesKeys: []string{"keys"}},
		{Key: "net", Name: "网络带宽", Unit: "bps", SeriesKeys: []string{"net_in", "net_out"}},
	}
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 Redis 在 Prometheus 中暂无数据（redis_exporter 未采集？）"
	}
	return res, nil
}

// aliyunRedisMetrics 阿里云 Redis(Tair/KVStore)：CMS acs_kvstore。关联键=resource_id(r-xxx)。
func (s *MetricsService) aliyunRedisMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "r-") {
		res.Message = "该 Redis 无阿里云实例 ID（resource_id），无法关联云监控"
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

	// 阿里云 Redis 的 CMS 数据按分片节点上报（instanceId=r-xxx-db-N），需不带维度拉取、
	// 按前缀过滤本实例的分片、再按节点聚合（%指标取 max，流量/计数取 sum）。
	metrics := []struct {
		key, name, unit, metric, agg string
	}{
		{"cpu", "CPU 使用率", "%", "CpuUsage", "max"},
		{"mem", "内存使用率", "%", "MemoryUsage", "max"},
		{"conn", "连接数使用率", "%", "ConnectionUsage", "max"},
		{"qps", "QPS", "/s", "UsedQPS", "sum"},
		{"hit", "命中率", "%", "ShardingHitRate", "avg"},
		{"keys", "键数量", "", "ShardingKeys", "sum"},
		{"net_in", "网络入", "bps", "IntranetIn", "sum"},
		{"net_out", "网络出", "bps", "IntranetOut", "sum"},
	}
	labelOf := map[string]string{"net_in": "入", "net_out": "出"}
	for _, m := range metrics {
		pts := cmsKvstoreSeries(ds, endpoint, instID, m.metric, startMs, endMs, period, m.agg)
		if len(pts) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Label: labelOf[m.key], Points: pts})
	}

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "conn", Name: "连接数使用率", Unit: "%", SeriesKeys: []string{"conn"}},
		{Key: "qps", Name: "QPS", Unit: "/s", SeriesKeys: []string{"qps"}},
		{Key: "hit", Name: "命中率", Unit: "%", SeriesKeys: []string{"hit"}},
		{Key: "keys", Name: "键数量", Unit: "", SeriesKeys: []string{"keys"}},
		{Key: "net", Name: "网络带宽", Unit: "bps", SeriesKeys: []string{"net_in", "net_out"}},
	}
	// 监控(历史)页深链需要实例规格段：实时查一次拿 InstanceClass，拿不到则回退实例详情页
	if cls := aliyunRedisClass(ds, region, instID); cls != "" {
		res.DetailURL = fmt.Sprintf("https://kvstore.console.aliyun.com/Redis/instance/%s/%s/Normal/VPC/%s/redisHistory", region, instID, cls)
	} else {
		res.DetailURL = fmt.Sprintf("https://kvstore.console.aliyun.com/Redis/instance/%s/%s", region, instID)
	}
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 Redis 在云监控中暂无数据"
	}
	return res, nil
}
