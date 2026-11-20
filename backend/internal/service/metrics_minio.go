package service

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// grafanaBase 从监控模板 URL 中取出 scheme://host 作为 Grafana 基址；无法解析返回空。
func grafanaBase(monitorURL string) string {
	monitorURL = strings.TrimSpace(monitorURL)
	if monitorURL == "" {
		return ""
	}
	u, err := url.Parse(monitorURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// minioMetrics MinIO 监控：自建对象存储，走 Prometheus（MinIO 自带 /minio/v2/metrics 指标）。
func (s *MetricsService) minioMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourcePrometheus {
		res.Message = "该来源暂不支持 MinIO 监控（" + ds.Type + "）"
		return res, nil
	}
	// MinIO 指标的 job / inst_name 标签即实例名（抓取时以实例名作 job）
	job := firstField(data, "inst_name")
	if job == "" {
		res.Message = "该 MinIO 无实例名，无法关联 Prometheus 指标"
		return res, nil
	}
	res.Target = job
	sel := fmt.Sprintf(`job="%s"`, job)
	totalB := fmt.Sprintf(`sum(minio_cluster_capacity_usable_total_bytes{%s})`, sel)
	freeB := fmt.Sprintf(`sum(minio_cluster_capacity_usable_free_bytes{%s})`, sel)

	queries := []struct {
		key, name, unit, label, expr string
	}{
		{"cap_pct", "容量使用率", "%", "", fmt.Sprintf(`(%s - %s) / %s * 100`, totalB, freeB, totalB)},
		{"cap_used", "已用容量", "bytes", "已用", fmt.Sprintf(`%s - %s`, totalB, freeB)},
		{"cap_total", "总容量", "bytes", "总量", totalB},
		{"objects", "对象数量", "count", "", fmt.Sprintf(`sum(minio_cluster_usage_object_total{%s})`, sel)},
		{"buckets", "桶数量", "count", "", fmt.Sprintf(`sum(minio_cluster_bucket_total{%s})`, sel)},
		{"nodes_online", "在线节点", "count", "节点", fmt.Sprintf(`sum(minio_cluster_nodes_online_total{%s})`, sel)},
		{"drives_online", "在线磁盘", "count", "磁盘", fmt.Sprintf(`sum(minio_cluster_drive_online_total{%s})`, sel)},
		{"req", "S3 请求速率", "/s", "请求", fmt.Sprintf(`sum(rate(minio_s3_requests_total{%s}[5m]))`, sel)},
		{"req_err", "S3 错误速率", "/s", "错误", fmt.Sprintf(`sum(rate(minio_s3_requests_errors_total{%s}[5m]))`, sel)},
	}

	end := time.Now().Unix()
	start := end - int64(lookback)
	client := noProxyClient(12 * time.Second)
	base := strings.TrimRight(ds.URL, "/")
	for _, q := range queries {
		pts, err := promQueryRange(client, base, ds, q.expr, start, end, step)
		if err != nil {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: q.key, Name: q.name, Unit: q.unit, Label: q.label, Points: pts})
	}

	res.Cards = []MetricCard{
		{Key: "cap_pct", Name: "容量使用率", Unit: "%", SeriesKeys: []string{"cap_pct"}},
		{Key: "cap", Name: "存储容量（已用/总）", Unit: "bytes", SeriesKeys: []string{"cap_used", "cap_total"}},
		{Key: "objects", Name: "对象数量", Unit: "count", SeriesKeys: []string{"objects"}},
		{Key: "buckets", Name: "桶数量", Unit: "count", SeriesKeys: []string{"buckets"}},
		{Key: "online", Name: "在线节点/磁盘", Unit: "count", SeriesKeys: []string{"nodes_online", "drives_online"}},
		{Key: "req", Name: "S3 请求速率（请求/错误）", Unit: "/s", SeriesKeys: []string{"req", "req_err"}},
	}

	// 「完整监控」优先指向 MinIO 专属 Grafana 看板（按实例 IP 过滤 var-instance）；
	// Grafana 基址复用数据源上配置的监控模板主机，取不到则回退 Prometheus 原生图表。
	ip := firstField(data, "resource_id", "instance_id")
	if ip == "" {
		ip = instIP(data)
	}
	if g := grafanaBase(ds.MonitorURL); g != "" && ip != "" {
		res.DetailURL = fmt.Sprintf("%s/d/TgmJnqnnk2/minio-dashboard-multi-instance?orgId=1&var-scrape_jobs=$__all&var-instance=%s", g, url.QueryEscape(ip))
	} else {
		capExpr := fmt.Sprintf(`(%s - %s) / %s * 100`, totalB, freeB, totalB)
		res.DetailURL = fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=0&g0.range_input=1h", base, url.QueryEscape(capExpr))
	}
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 MinIO 在 Prometheus 中暂无监控数据"
	}
	return res, nil
}
