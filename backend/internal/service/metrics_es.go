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

// esQuery 查询 acs_elasticsearch 指标（维度 clusterId，可按 groupBy=nodeIP 分组）。
func esQuery(ds *model.DataSource, endpoint, metric, clusterID string, startMs, endMs int64, period, groupBy string) []labeledSeries {
	extra := map[string]string{
		"Namespace":  "acs_elasticsearch",
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
		"Dimensions": fmt.Sprintf(`{"clusterId":"%s"}`, clusterID),
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
	byGroup := map[string][][2]float64{}
	for _, dp := range dps {
		ts, ok := cmsNum(dp["timestamp"])
		if !ok {
			continue
		}
		val, ok := cmsDatapointVal(dp)
		if !ok {
			continue
		}
		label := ""
		if groupBy != "" {
			label = fmt.Sprint(dp[groupBy])
		}
		byGroup[label] = append(byGroup[label], [2]float64{ts / 1000.0, val})
	}
	list := make([]labeledSeries, 0, len(byGroup))
	for lbl, pts := range byGroup {
		sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
		list = append(list, labeledSeries{label: lbl, points: pts})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].label < list[j].label })
	return list
}

// elasticMetrics Elasticsearch 监控：阿里云走 CMS acs_elasticsearch（关联键=resource_id=clusterId）。
func (s *MetricsService) elasticMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourceAliyun {
		res.Message = "该来源暂不支持 Elasticsearch 监控（" + ds.Type + "）"
		return res, nil
	}
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "es-") {
		res.Message = "该 ES 无阿里云实例 ID（resource_id），无法关联云监控"
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

	// 分节点：每个节点一条线，label 去掉集群前缀更短（es-cn-xxx-data-g-0 → data-g-0）
	addPerNode := func(key, name, unit, metric string) {
		for _, g := range esQuery(ds, endpoint, metric, instID, startMs, endMs, period, "nodeIP") {
			if len(g.points) == 0 {
				continue
			}
			label := strings.TrimPrefix(g.label, instID+"-")
			if label == "" {
				label = "node"
			}
			res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: g.points})
		}
	}
	// 集群级：单线
	addCluster := func(key, name, unit, label, metric string) {
		g := esQuery(ds, endpoint, metric, instID, startMs, endMs, period, "")
		if len(g) == 0 || len(g[0].points) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: g[0].points})
	}

	addPerNode("cpu", "CPU 使用率", "%", "NodeCPUUtilization")
	addPerNode("heap", "JVM 堆内存使用率", "%", "NodeHeapMemoryUtilization")
	addPerNode("disk", "磁盘使用率", "%", "NodeDiskUtilization")
	addCluster("status", "集群健康度", "", "", "ClusterStatus")
	addCluster("q_qps", "查询QPS", "/s", "查询", "ClusterQueryQPS")
	addCluster("w_qps", "写入QPS", "/s", "写入", "ClusterIndexQPS")
	addCluster("q_lat", "搜索延时", "ms", "搜索", "ClusterSearchLatency")
	addCluster("w_lat", "写入延时", "ms", "写入", "ClusterIndexingLatency")
	addCluster("http2", "2xx", "/s", "2xx", "ClusterRequest2xxQPS")
	addCluster("http4", "4xx", "/s", "4xx", "ClusterRequest4xxQPS")
	addCluster("http5", "5xx", "/s", "5xx", "ClusterRequest5xxQPS")
	addCluster("free", "可用存储空间", "bytes", "", "ClusterFreeStorageSpace")
	addCluster("docs", "可搜索文档数", "count", "", "ClusterSearchableDocuments")

	res.Cards = []MetricCard{
		{Key: "status", Name: "集群健康度（0正常/1黄/2红）", Unit: "", SeriesKeys: []string{"status"}},
		{Key: "cpu", Name: "CPU 使用率（分节点）", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "heap", Name: "JVM 堆内存（分节点）", Unit: "%", SeriesKeys: []string{"heap"}},
		{Key: "disk", Name: "磁盘使用率（分节点）", Unit: "%", SeriesKeys: []string{"disk"}},
		{Key: "qps", Name: "QPS（查询/写入）", Unit: "/s", SeriesKeys: []string{"q_qps", "w_qps"}},
		{Key: "lat", Name: "延时（搜索/写入）", Unit: "ms", SeriesKeys: []string{"q_lat", "w_lat"}},
		{Key: "http", Name: "HTTP 请求（2xx/4xx/5xx）", Unit: "/s", SeriesKeys: []string{"http2", "http4", "http5"}},
		{Key: "free", Name: "可用存储空间", Unit: "bytes", SeriesKeys: []string{"free"}},
		{Key: "docs", Name: "可搜索文档数", Unit: "count", SeriesKeys: []string{"docs"}},
	}
	res.DetailURL = fmt.Sprintf("https://elasticsearch.console.aliyun.com/%s/instances/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 Elasticsearch 在云监控中暂无数据"
	}
	return res, nil
}
