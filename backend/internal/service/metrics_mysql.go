package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// mysqlMetrics MySQL 实例监控：按来源路由（自建→mysqld_exporter，阿里云→RDS 云监控）。
func (s *MetricsService) mysqlMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	switch ds.Type {
	case model.DataSourcePrometheus:
		return s.promMysqlMetrics(ds, data, lookback, step, res)
	case model.DataSourceAliyun:
		return s.aliyunRdsMetrics(ds, data, lookback, step, res)
	case model.DataSourceTencent:
		res.Message = "腾讯云 MySQL 监控对接中"
		return res, nil
	default:
		res.Message = "该来源暂不支持 MySQL 监控（" + ds.Type + "）"
		return res, nil
	}
}

// —— 自建 MySQL：mysqld_exporter ——

// promMysqlMetrics 用 mysqld_exporter 指标画 连接数/QPS/慢查询/网络。
// 关联键：实例的 endpoint（IP）→ exporter 的 instance 标签。
func (s *MetricsService) promMysqlMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	ip := firstField(data, "endpoint", "bk_host_innerip", "ip", "host_ip")
	// endpoint 可能带端口或是域名，取 IP 部分
	if i := strings.IndexAny(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	if ip == "" {
		res.Message = "该 MySQL 无 endpoint/IP，无法关联监控"
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
	// 主机 CPU/内存：mysqld_exporter 不含，取同机 node_exporter（endpoint IP 命中则有）
	add("cpu", "CPU 使用率", "%", "", fmt.Sprintf(`100 - (avg(rate(node_cpu_seconds_total{%s,mode="idle"}[5m])) * 100)`, sel))
	add("mem", "内存使用率", "%", "", fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{%s} / node_memory_MemTotal_bytes{%s}) * 100`, sel, sel))
	add("conn_cur", "连接数", "", "当前", fmt.Sprintf(`mysql_global_status_threads_connected{%s}`, sel))
	add("conn_max", "连接数", "", "上限", fmt.Sprintf(`mysql_global_variables_max_connections{%s}`, sel))
	add("qps", "QPS", "/s", "", fmt.Sprintf(`rate(mysql_global_status_queries{%s}[5m])`, sel))
	add("tps", "TPS", "/s", "", fmt.Sprintf(`rate(mysql_global_status_com_commit{%s}[5m]) + rate(mysql_global_status_com_rollback{%s}[5m])`, sel, sel))
	add("slow", "慢查询", "/s", "", fmt.Sprintf(`rate(mysql_global_status_slow_queries{%s}[5m])`, sel))
	add("net_in", "网络入", "bps", "入", fmt.Sprintf(`rate(mysql_global_status_bytes_received{%s}[5m]) * 8`, sel))
	add("net_out", "网络出", "bps", "出", fmt.Sprintf(`rate(mysql_global_status_bytes_sent{%s}[5m]) * 8`, sel))

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "conn", Name: "连接数", Unit: "", SeriesKeys: []string{"conn_cur", "conn_max"}},
		{Key: "qps", Name: "QPS / TPS", Unit: "/s", SeriesKeys: []string{"qps", "tps"}},
		{Key: "slow", Name: "慢查询", Unit: "/s", SeriesKeys: []string{"slow"}},
		{Key: "net", Name: "网络带宽", Unit: "bps", SeriesKeys: []string{"net_in", "net_out"}},
	}
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 MySQL 在 Prometheus 中暂无监控数据（mysqld_exporter 未采集？）"
	}
	return res, nil
}

// —— 阿里云 RDS：云监控 acs_rds_dashboard ——

// aliyunRdsMetrics 用 RDS 云监控画 CPU/内存/磁盘/连接数/QPS/TPS。
// 关联键：resource_id（rm-xxx）。
func (s *MetricsService) aliyunRdsMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "rm-") {
		res.Message = "该 MySQL 无阿里云 RDS 实例 ID（resource_id），无法关联云监控"
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
		key, name, unit, metric string
	}{
		{"cpu", "CPU 使用率", "%", "CpuUsage"},
		{"mem", "内存使用率", "%", "MemoryUsage"},
		{"disk", "磁盘使用率", "%", "DiskUsage"},
		{"iops", "IOPS 使用率", "%", "IOPSUsage"},
		{"conn", "连接数使用率", "%", "ConnectionUsage"},
		{"qps", "QPS", "/s", "MySQL_QPS"},
		{"tps", "TPS", "/s", "MySQL_TPS"},
	}
	for _, m := range metrics {
		groups, err := cmsMetric(ds, endpoint, "acs_rds_dashboard", instID, m.metric, startMs, endMs, period, "")
		if err != nil || len(groups) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: m.key, Name: m.name, Unit: m.unit, Points: groups[0].points})
	}

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "disk", Name: "磁盘使用率", Unit: "%", SeriesKeys: []string{"disk"}},
		{Key: "iops", Name: "IOPS 使用率", Unit: "%", SeriesKeys: []string{"iops"}},
		{Key: "conn", Name: "连接数使用率", Unit: "%", SeriesKeys: []string{"conn"}},
		{Key: "qps", Name: "QPS / TPS", Unit: "/s", SeriesKeys: []string{"qps", "tps"}},
	}
	// 阿里云 RDS 控制台监控页深链
	res.DetailURL = fmt.Sprintf("https://rdsnext.console.aliyun.com/detail/%s/monitorAlarm?region=%s", instID, region)

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 RDS 在云监控中暂无数据"
	}
	return res, nil
}
