package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// lindormMetrics Lindorm 监控：阿里云走 CMS acs_lindorm（指标按节点 host 上报，按 instanceId 聚合为实例级）。
func (s *MetricsService) lindormMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	if ds.Type != model.DataSourceAliyun {
		res.Message = "该来源暂不支持 Lindorm 监控（" + ds.Type + "）"
		return res, nil
	}
	instID := firstField(data, "resource_id", "instance_id")
	if instID == "" || !strings.HasPrefix(instID, "ld-") {
		res.Message = "该 Lindorm 无阿里云实例 ID（resource_id），无法关联云监控"
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

	// agg: 把各节点(host)的点按时间戳聚合为单条序列（sum/avg/max）
	agg := func(metric, mode string) [][2]float64 {
		groups, err := cmsMetric(ds, endpoint, "acs_lindorm", instID, metric, startMs, endMs, period, "host")
		if err != nil || len(groups) == 0 {
			return nil
		}
		type acc struct {
			sum, max float64
			n        int
			hasMax   bool
		}
		m := map[float64]*acc{}
		for _, g := range groups {
			for _, p := range g.points {
				a := m[p[0]]
				if a == nil {
					a = &acc{}
					m[p[0]] = a
				}
				a.sum += p[1]
				a.n++
				if !a.hasMax || p[1] > a.max {
					a.max, a.hasMax = p[1], true
				}
			}
		}
		pts := make([][2]float64, 0, len(m))
		for t, a := range m {
			var v float64
			switch mode {
			case "avg":
				if a.n > 0 {
					v = a.sum / float64(a.n)
				}
			case "max":
				v = a.max
			default:
				v = a.sum
			}
			pts = append(pts, [2]float64{t, v})
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i][0] < pts[j][0] })
		return pts
	}
	// 集群汇总单线（容量/吞吐/请求/连接）
	add := func(key, name, unit, label string, pts [][2]float64) {
		if len(pts) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
	}
	// 分节点多线（CPU/内存/负载/磁盘）：一张卡每个节点一条线，label=节点名；transform 可对每个值变换。
	addPerNode := func(key, name, unit, metric string, transform func(float64) float64) {
		groups, err := cmsMetric(ds, endpoint, "acs_lindorm", instID, metric, startMs, endMs, period, "host")
		if err != nil {
			return
		}
		for _, g := range groups {
			if len(g.points) == 0 {
				continue
			}
			pts := g.points
			if transform != nil {
				for i := range pts {
					pts[i][1] = transform(pts[i][1])
				}
			}
			label := g.label
			if label == "" {
				label = "node"
			}
			res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
		}
	}

	// 分节点：CPU 使用率(=100-空闲)、内存使用率、系统负载、磁盘读/写
	addPerNode("cpu", "CPU 使用率", "%", "cpu_idle", func(v float64) float64 { return 100 - v })
	addPerNode("mem", "内存使用率", "%", "mem_used_percent", nil)
	addPerNode("load", "系统负载", "", "load_one", nil)
	addPerNode("disk_r", "磁盘读", "Bps", "disk_readbytes", nil)
	addPerNode("disk_w", "磁盘写", "Bps", "disk_writebytes", nil)
	// 集群汇总：存储使用率/容量、网络、请求、连接
	add("hot_pct", "热存储使用率", "%", "热", agg("hot_storage_used_percent", "max"))
	add("cold_pct", "冷存储使用率", "%", "冷", agg("cold_storage_used_percent", "max"))
	add("hot_used", "热存储已用", "bytes", "已用", agg("hot_storage_used_bytes", "sum"))
	add("hot_total", "热存储总量", "bytes", "总量", agg("hot_storage_total_bytes", "sum"))
	add("net_in", "网络流入", "Bps", "流入", agg("bytes_in", "sum"))
	add("net_out", "网络流出", "Bps", "流出", agg("bytes_out", "sum"))
	add("get_ops", "Get", "/s", "Get", agg("get_num_ops", "sum"))
	add("put_ops", "Put", "/s", "Put", agg("put_num_ops", "sum"))
	add("scan_ops", "Scan", "/s", "Scan", agg("scan_num_ops", "sum"))
	add("conn", "SQL 连接数", "count", "", agg("lql_connection", "sum"))

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率（分节点）", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率（分节点）", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "load", Name: "系统负载（分节点）", Unit: "", SeriesKeys: []string{"load"}},
		{Key: "disk_r", Name: "磁盘读（分节点）", Unit: "Bps", SeriesKeys: []string{"disk_r"}},
		{Key: "disk_w", Name: "磁盘写（分节点）", Unit: "Bps", SeriesKeys: []string{"disk_w"}},
		{Key: "storage_pct", Name: "存储使用率（热/冷）", Unit: "%", SeriesKeys: []string{"hot_pct", "cold_pct"}},
		{Key: "storage", Name: "热存储容量（已用/总）", Unit: "bytes", SeriesKeys: []string{"hot_used", "hot_total"}},
		{Key: "net", Name: "网络流量（入/出，集群）", Unit: "Bps", SeriesKeys: []string{"net_in", "net_out"}},
		{Key: "ops", Name: "请求速率（Get/Put/Scan，集群）", Unit: "/s", SeriesKeys: []string{"get_ops", "put_ops", "scan_ops"}},
		{Key: "conn", Name: "SQL 连接数（集群）", Unit: "count", SeriesKeys: []string{"conn"}},
	}
	res.DetailURL = fmt.Sprintf("https://lindorm.console.aliyun.com/%s/cluster/lindorm/%s/monitor", region, instID)
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该 Lindorm 在云监控中暂无数据"
	}
	return res, nil
}
