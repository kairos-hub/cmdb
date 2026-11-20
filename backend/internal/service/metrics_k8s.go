package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// k8sMetrics Kubernetes 集群监控：
//   - Prometheus 来源（集群自带 kube-state-metrics + cadvisor/kubelet）：拉取集群级指标
//     （节点/CPU/内存/Pod/工作负载/网络），实时查询，不入库。
//   - 阿里云 ACK 来源：走云监控 acs_k8s（维度 cluster），集群级 + 各节点指标。
func (s *MetricsService) k8sMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	cluster := firstField(data, "cluster_name", "inst_name")
	res.Target = cluster

	if ds.Type == model.DataSourceAliyun {
		return s.ackMetrics(ds, data, lookback, step, res)
	}
	if ds.Type != model.DataSourcePrometheus {
		res.Supported = true
		res.Message = "该集群未绑定 Prometheus 监控数据源（当前来源：" + ds.Type + "）。"
		return res, nil
	}

	base := strings.TrimRight(ds.URL, "/")
	if base == "" {
		res.Message = "该集群绑定的 Prometheus 数据源未配置地址"
		return res, nil
	}
	client := noProxyClient(12 * time.Second)
	s.promK8sFill(client, base, ds, lookback, step, res)

	// 深链：数据源配了 Grafana 看板地址(monitor_url，集群级完整看板)则优先用；否则回退 Prometheus 图形界面。
	if mu := strings.TrimSpace(ds.MonitorURL); mu != "" {
		res.DetailURL = mu
	} else {
		cpuExpr := `sum(rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m])) / sum(kube_node_status_capacity{resource="cpu"}) * 100`
		res.DetailURL = fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=0&g0.range_input=1h", base, url.QueryEscape(cpuExpr))
	}

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该集群在 Prometheus 中暂无监控数据"
	}
	return res, nil
}

// promK8sFill 用一个 Prometheus 查询地址填充 k8s 集群级 + 各节点指标的 series 与 cards
// （不设置 DetailURL/Supported/Message）。自建集群与阿里云 ACK 的 ARMS Prometheus 共用此逻辑。
func (s *MetricsService) promK8sFill(client *http.Client, base string, ds *model.DataSource, lookback, step int, res *HostMetricsResult) {
	base = strings.TrimRight(base, "/")
	end := time.Now().Unix()
	start := end - int64(lookback)

	// CPU/内存使用率优先用 node-exporter 整机口径（含系统/kubelet/OS，与 ARMS 看板/控制台一致）；
	// 无 node-exporter 的集群（如部分自建）回退「容器工作集 / 节点容量」口径（偏低但可用）。
	// nodeNE: 把 node-exporter 的 instance(节点IP:9100) 经 kube_node_info.internal_ip 映射回节点名，并 ×100。
	nodeNE := func(inner string) string {
		return `max by(node)( max by(ip)( label_replace((` + inner + `),"ip","$1","instance","(.*):.*") ) * on(ip) group_left(node) max by(ip,node)( label_replace(kube_node_info,"ip","$1","internal_ip","(.*)") ) ) * 100`
	}

	// 单线指标
	type q struct{ key, name, unit, expr, label string }
	singles := []q{
		// 节点
		{"node_total", "节点总数", "count", `count(kube_node_info)`, "总数"},
		{"node_ready", "就绪节点", "count", `sum(kube_node_status_condition{condition="Ready",status="true"})`, "就绪"},
		// Pod 状态
		{"pod_running", "运行中", "count", `sum(kube_pod_status_phase{phase="Running"})`, "运行中"},
		{"pod_pending", "待调度", "count", `sum(kube_pod_status_phase{phase="Pending"})`, "待调度"},
		{"pod_failed", "失败", "count", `sum(kube_pod_status_phase{phase="Failed"})`, "失败"},
		// 工作负载副本
		{"rep_desired", "期望副本", "count", `sum(kube_deployment_spec_replicas)`, "期望"},
		{"rep_avail", "可用副本", "count", `sum(kube_deployment_status_replicas_available)`, "可用"},
		// 容器网络（bit/s）
		{"net_in", "网络入", "bps", `sum(rate(container_network_receive_bytes_total[5m])) * 8`, "入"},
		{"net_out", "网络出", "bps", `sum(rate(container_network_transmit_bytes_total[5m])) * 8`, "出"},
		// 容器重启（近 5m 增量）
		{"restarts", "容器重启", "count", `sum(increase(kube_pod_container_status_restarts_total[5m]))`, ""},
	}
	for _, it := range singles {
		pts, err := promQueryRange(client, base, ds, it.expr, start, end, step)
		if err != nil || len(pts) == 0 {
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: it.key, Name: it.name, Unit: it.unit, Label: it.label, Points: pts})
	}

	// 集群级 CPU/内存：node-exporter 整机口径优先，容器口径兜底。
	addSinglePref := func(key, name, primary, fallback string) {
		pts, err := promQueryRange(client, base, ds, primary, start, end, step)
		if err != nil || len(pts) == 0 {
			pts, err = promQueryRange(client, base, ds, fallback, start, end, step)
			if err != nil || len(pts) == 0 {
				return
			}
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: "%", Points: pts})
	}
	addSinglePref("cpu", "CPU 使用率",
		`(1 - sum(rate(node_cpu_seconds_total{mode="idle"}[5m])) / sum(rate(node_cpu_seconds_total[5m]))) * 100`,
		`sum(rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m])) / sum(kube_node_status_capacity{resource="cpu"}) * 100`)
	addSinglePref("mem", "内存使用率",
		`(1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)) * 100`,
		`sum(container_memory_working_set_bytes{container!="",image!=""}) / sum(kube_node_status_capacity{resource="memory"}) * 100`)

	// 各节点多线（按 node 拆线，追加前按最新值高→低排序）。primary 为空时用 fallback。
	addPromNode := func(key, name, primary, fallback string) {
		series, err := promQueryRangeMulti(client, base, ds, primary, start, end, step, "node")
		if (err != nil || len(series) == 0) && fallback != "" {
			series, err = promQueryRangeMulti(client, base, ds, fallback, start, end, step, "node")
		}
		if err != nil {
			return
		}
		tmp := make([]MetricSeries, 0, len(series))
		for _, g := range series {
			if len(g.points) == 0 {
				continue
			}
			tmp = append(tmp, MetricSeries{Key: key, Name: name, Unit: "%", Label: g.label, Points: g.points})
		}
		sortSeriesByLastDesc(tmp)
		res.Series = append(res.Series, tmp...)
	}
	// 各节点 CPU/内存：node-exporter 整机口径优先（与集群卡一致），容器/容量口径兜底。
	addPromNode("node_cpu", "节点 CPU 使用率",
		nodeNE(`1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m]))`),
		`sum by(node)(rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m])) / sum by(node)(kube_node_status_capacity{resource="cpu"}) * 100`)
	addPromNode("node_mem", "节点内存使用率",
		nodeNE(`1 - node_memory_MemAvailable_bytes/node_memory_MemTotal_bytes`),
		`sum by(node)(container_memory_working_set_bytes{container!="",image!=""}) / sum by(node)(kube_node_status_capacity{resource="memory"}) * 100`)
	// 节点磁盘使用率（根分区 /，(总量-可用)/总量）。依赖 node-exporter；无则该卡自动隐藏（pruneEmptyCards）。
	addPromNode("node_disk", "节点磁盘使用率",
		nodeNE(`1 - node_filesystem_avail_bytes{mountpoint="/",fstype!="tmpfs"} / node_filesystem_size_bytes{mountpoint="/",fstype!="tmpfs"}`), "")

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "node", Name: "节点（就绪/总数）", Unit: "count", SeriesKeys: []string{"node_ready", "node_total"}},
		{Key: "pod", Name: "Pod 状态（运行/待调度/失败）", Unit: "count", SeriesKeys: []string{"pod_running", "pod_pending", "pod_failed"}},
		{Key: "rep", Name: "工作负载副本（期望/可用）", Unit: "count", SeriesKeys: []string{"rep_desired", "rep_avail"}},
		{Key: "net", Name: "容器网络（入/出）", Unit: "bps", SeriesKeys: []string{"net_in", "net_out"}},
		{Key: "restarts", Name: "容器重启（近5m增量）", Unit: "count", SeriesKeys: []string{"restarts"}},
		{Key: "node_cpu", Name: "各节点 CPU 使用率", Unit: "%", SeriesKeys: []string{"node_cpu"}},
		{Key: "node_mem", Name: "各节点内存使用率", Unit: "%", SeriesKeys: []string{"node_mem"}},
		{Key: "node_disk", Name: "各节点磁盘使用率", Unit: "%", SeriesKeys: []string{"node_disk"}},
	}
}

// armsBaseCache 缓存集群 → ARMS Prometheus 查询地址（含内嵌 token），TTL 10 分钟。
var armsBaseCache sync.Map

type armsBaseEntry struct {
	url string
	exp time.Time
}

// armsReachable 用短超时对某 Prometheus 查询地址做一次轻量连通性探测（vector(1)），用于判断内网端点是否可达。
func armsReachable(base string) bool {
	client := noProxyClient(3 * time.Second)
	end := time.Now().Unix()
	_, err := promQueryRange(client, strings.TrimRight(base, "/"), &model.DataSource{Type: model.DataSourcePrometheus}, `vector(1)`, end-60, end, 60)
	return err == nil
}

// armsPromBase 用账号 AK/SK 查询某 ACK 集群的 ARMS Prometheus HTTP API 查询地址（URL 内嵌 token、免密）。
// 内网端点优先（省公网流量）：先对内网地址做一次短超时连通性探测，通则用内网、不通自动回退公网。
// 选定结果缓存 10 分钟，探测每 10 分钟每集群最多一次，不拖慢每次监控请求。查不到/无 ARMS 实例返回空串。
func (s *MetricsService) armsPromBase(ds *model.DataSource, clusterID, region string) string {
	return armsPromBaseURL(ds, clusterID, region)
}

// ackClusterInfo 按集群名(同源 kubernetes 实例)取其 ACK 集群ID(resource_id)与地域。
func (s *MetricsService) ackClusterInfo(ds *model.DataSource, clusterName string) (rid, region string) {
	if strings.TrimSpace(clusterName) == "" {
		return "", ""
	}
	var m model.Model
	if s.db.Where("obj_id = ?", "kubernetes").First(&m).Error != nil {
		return "", ""
	}
	var inst model.Instance
	if s.db.Where("model_id = ? AND source_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name')) = ?",
		m.ID, ds.ID, clusterName).First(&inst).Error != nil {
		return "", ""
	}
	var d map[string]interface{}
	if json.Unmarshal(inst.Data, &d) != nil {
		return "", ""
	}
	rid, _ = d["resource_id"].(string)
	region, _ = d["region"].(string)
	return rid, region
}

// armsPromBaseURL 同 armsPromBase 的包级函数（供非 MetricsService 复用，如 ACK 应用同步）。
func armsPromBaseURL(ds *model.DataSource, clusterID, region string) string {
	if clusterID == "" || strings.TrimSpace(ds.Username) == "" {
		return ""
	}
	if v, ok := armsBaseCache.Load(clusterID); ok {
		if e := v.(armsBaseEntry); time.Now().Before(e.exp) {
			return e.url
		}
	}
	if region == "" {
		region = "cn-hangzhou"
	}
	body, err := aliRPC("https://arms."+region+".aliyuncs.com", ds.Username, ds.Password, "2019-08-08",
		"GetPrometheusInstance", map[string]string{"ClusterId": clusterID, "RegionId": region})
	if err != nil {
		return ""
	}
	var o struct {
		Data struct {
			HttpApiInterUrl string `json:"HttpApiInterUrl"`
			HttpApiIntraUrl string `json:"HttpApiIntraUrl"`
		} `json:"Data"`
	}
	if json.Unmarshal(body, &o) != nil {
		return ""
	}
	intra := strings.TrimSpace(o.Data.HttpApiIntraUrl)
	inter := strings.TrimSpace(o.Data.HttpApiInterUrl)
	chosen := ""
	if intra != "" && armsReachable(intra) { // 内网可达 → 优先内网，省公网流量
		chosen = intra
	}
	if chosen == "" {
		chosen = inter // 内网不通 → 公网兜底
	}
	if chosen == "" {
		chosen = intra // 仅有内网地址时的最后兜底
	}
	if chosen != "" {
		armsBaseCache.Store(clusterID, armsBaseEntry{url: chosen, exp: time.Now().Add(10 * time.Minute)})
	}
	return chosen
}

// ackProfileCache 缓存集群 → profile（Edge/Default/…），TTL 1 小时（profile 基本不变）。
var ackProfileCache sync.Map

type ackProfileEntry struct {
	profile string
	exp     time.Time
}

// ackClusterProfile 查询 ACK 集群 profile（边缘集群=Edge，标准托管=Default）。
// 边缘集群的 cadvisor 指标 ARMS 抓取不全，需对其保留云监控 acs_k8s 优先，故按此信号分流。
func (s *MetricsService) ackClusterProfile(ds *model.DataSource, clusterID string) string {
	if clusterID == "" || strings.TrimSpace(ds.Username) == "" {
		return ""
	}
	if v, ok := ackProfileCache.Load(clusterID); ok {
		if e := v.(ackProfileEntry); time.Now().Before(e.exp) {
			return e.profile
		}
	}
	body, err := aliROA("https://cs.aliyuncs.com", "/clusters/"+clusterID, nil, ds.Username, ds.Password, "2015-12-15")
	if err != nil {
		return ""
	}
	var c struct {
		Profile string `json:"profile"`
	}
	if json.Unmarshal(body, &c) != nil {
		return ""
	}
	ackProfileCache.Store(clusterID, ackProfileEntry{profile: c.Profile, exp: time.Now().Add(time.Hour)})
	return c.Profile
}

// sortSeriesByLastDesc 按每条序列最新值由高到低排序（空序列排末尾），用于节点线按使用率高→低展示。
func sortSeriesByLastDesc(ss []MetricSeries) {
	last := func(s MetricSeries) float64 {
		if len(s.Points) == 0 {
			return -1e18
		}
		return s.Points[len(s.Points)-1][1]
	}
	sort.SliceStable(ss, func(i, j int) bool { return last(ss[i]) > last(ss[j]) })
}

// ackMetric 调用云监控 acs_k8s（维度 cluster）取单指标；groupBy（如 node）非空时按该标签拆多条线。
func ackMetric(ds *model.DataSource, endpoint, clusterID, metric string, startMs, endMs int64, period, groupBy string) ([]labeledSeries, error) {
	extra := map[string]string{
		"Namespace":  "acs_k8s",
		"MetricName": metric,
		"StartTime":  strconv.FormatInt(startMs, 10),
		"EndTime":    strconv.FormatInt(endMs, 10),
		"Period":     period,
		"Dimensions": fmt.Sprintf(`{"cluster":"%s"}`, clusterID),
	}
	body, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-01-01", "DescribeMetricList", extra)
	if err != nil {
		return nil, err
	}
	var out struct {
		Datapoints string `json:"Datapoints"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Datapoints) == "" {
		return []labeledSeries{}, nil
	}
	var dps []map[string]interface{}
	if err := json.Unmarshal([]byte(out.Datapoints), &dps); err != nil {
		return nil, err
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
	return list, nil
}

// ackMetrics 阿里云 ACK 集群监控：云监控 acs_k8s 集群级 + 各节点指标。
func (s *MetricsService) ackMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	rid := firstField(data, "resource_id")
	region := firstField(data, "region", "bk_region")
	if region == "" {
		region = parseRegions(ds.Region)[0]
	}
	res.Supported = true
	// ACK 控制台监控页：#/k8s/cluster/<id>/v2/info/overview?tab=monitor&clusterType=...&region=...
	// 页面按集群 ID 自解析类型，clusterType 对标准托管/边缘集群均为 ManagedKubernetes；profile/ns/state 等为非必需 UI 状态，省略。
	res.DetailURL = fmt.Sprintf("https://cs.console.aliyun.com/#/k8s/cluster/%s/v2/info/overview?tab=monitor&clusterType=ManagedKubernetes&region=%s", rid, region)
	if rid == "" {
		res.Message = "该 ACK 集群无集群 ID（resource_id），无法关联云监控"
		return res, nil
	}

	// 边缘集群（profile=Edge）的边缘节点 cadvisor 指标 ARMS 抓取不全，保留云监控 acs_k8s 优先；
	// 其余标准托管集群优先走 ARMS Prometheus（与自建集群同口径、卡片更全，acs_k8s 已被阿里云弃用）。
	edge := s.ackClusterProfile(ds, rid) == "Edge"
	if !edge {
		if base := s.armsPromBase(ds, rid, region); base != "" {
			armsDS := &model.DataSource{Type: model.DataSourcePrometheus, URL: base}
			s.promK8sFill(noProxyClient(15*time.Second), base, armsDS, lookback, step, res)
			if len(res.Series) > 0 {
				res.SourceName = strings.TrimSpace(ds.Name + " · ARMS Prometheus")
				return res, nil
			}
			res.Series = res.Series[:0] // ARMS 无数据：清空后回退 acs_k8s
		}
	}

	endpoint := cmsEndpoint(region)
	endMs := time.Now().UnixMilli()
	startMs := endMs - int64(lookback)*1000
	period := strconv.Itoa(step)

	// 集群级单线
	addC := func(key, name, unit, label, metric string) {
		g, err := ackMetric(ds, endpoint, rid, metric, startMs, endMs, period, "")
		if err != nil || len(g) == 0 || len(g[0].points) == 0 {
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: g[0].points})
	}
	// 各节点多线（直接取单指标），按最新值由高到低排序后追加。
	addNode := func(key, name, unit, metric string) {
		groups, err := ackMetric(ds, endpoint, rid, metric, startMs, endMs, period, "node")
		if err != nil {
			return
		}
		tmp := make([]MetricSeries, 0, len(groups))
		for _, g := range groups {
			if len(g.points) == 0 {
				continue
			}
			tmp = append(tmp, MetricSeries{Key: key, Name: name, Unit: unit, Label: g.label, Points: g.points})
		}
		sortSeriesByLastDesc(tmp)
		res.Series = append(res.Series, tmp...)
	}
	// 各节点多线（分子/分母两指标，按节点、按时间点相除 ×scale）。
	// 用于内存使用率走「物理总量」口径（working_set / capacity），与 ACK Prometheus 看板一致、≤100%；
	// 而非云监控原生的 node.memory.utilization（working_set / allocatable，可超 100%）。
	addNodeRatio := func(key, name, unit, numMetric, denMetric string, scale float64) {
		num, err := ackMetric(ds, endpoint, rid, numMetric, startMs, endMs, period, "node")
		if err != nil || len(num) == 0 {
			return
		}
		den, err := ackMetric(ds, endpoint, rid, denMetric, startMs, endMs, period, "node")
		if err != nil {
			return
		}
		// den: node -> (ts -> val)，并记录每节点最后一个值兜底（capacity 近似恒定）
		dmap := map[string]map[float64]float64{}
		dlast := map[string]float64{}
		for _, g := range den {
			m := map[float64]float64{}
			for _, p := range g.points {
				m[p[0]] = p[1]
				if p[1] > 0 {
					dlast[g.label] = p[1]
				}
			}
			dmap[g.label] = m
		}
		tmp := make([]MetricSeries, 0, len(num))
		for _, g := range num {
			dm := dmap[g.label]
			pts := make([][2]float64, 0, len(g.points))
			for _, p := range g.points {
				dv, ok := dm[p[0]]
				if !ok || dv == 0 {
					dv = dlast[g.label] // 时间点对不齐时用该节点 capacity 兜底
				}
				if dv == 0 {
					continue
				}
				pts = append(pts, [2]float64{p[0], p[1] / dv * scale})
			}
			if len(pts) == 0 {
				continue
			}
			tmp = append(tmp, MetricSeries{Key: key, Name: name, Unit: unit, Label: g.label, Points: pts})
		}
		sortSeriesByLastDesc(tmp)
		res.Series = append(res.Series, tmp...)
	}
	// 各节点「使用率 = (总量-可用)/总量」多线（如磁盘：limit/available），按节点、按时间点计算，高→低排序。
	addNodeUsedRatio := func(key, name, availMetric, limitMetric string) {
		avail, e1 := ackMetric(ds, endpoint, rid, availMetric, startMs, endMs, period, "node")
		limit, e2 := ackMetric(ds, endpoint, rid, limitMetric, startMs, endMs, period, "node")
		if e1 != nil || e2 != nil || len(avail) == 0 || len(limit) == 0 {
			return
		}
		lmap := map[string]map[float64]float64{}
		llast := map[string]float64{}
		for _, g := range limit {
			m := map[float64]float64{}
			for _, p := range g.points {
				m[p[0]] = p[1]
				if p[1] > 0 {
					llast[g.label] = p[1]
				}
			}
			lmap[g.label] = m
		}
		tmp := make([]MetricSeries, 0, len(avail))
		for _, g := range avail {
			lm := lmap[g.label]
			pts := make([][2]float64, 0, len(g.points))
			for _, p := range g.points {
				lv, ok := lm[p[0]]
				if !ok || lv == 0 {
					lv = llast[g.label] // 总量近似恒定，时间点对不齐时兜底
				}
				if lv == 0 {
					continue
				}
				pts = append(pts, [2]float64{p[0], (lv - p[1]) / lv * 100}) // (总量-可用)/总量
			}
			if len(pts) == 0 {
				continue
			}
			tmp = append(tmp, MetricSeries{Key: key, Name: name, Unit: "%", Label: g.label, Points: pts})
		}
		sortSeriesByLastDesc(tmp)
		res.Series = append(res.Series, tmp...)
	}
	// 集群内存使用率走物理总量口径：Σ节点 working_set / Σ节点 capacity（与节点卡一致、≤100%）。
	// 云监控原生 cluster.memory.utilization 口径是 usage/allocatable（与节点卡不一致），仅在节点分量取不到时兜底。
	addClusterMemPhysical := func() {
		num, e1 := ackMetric(ds, endpoint, rid, "node.memory.working_set", startMs, endMs, period, "node")
		den, e2 := ackMetric(ds, endpoint, rid, "node.memory.capacity", startMs, endMs, period, "node")
		if e1 != nil || e2 != nil || len(num) == 0 || len(den) == 0 {
			addC("mem", "内存使用率", "%", "", "cluster.memory.utilization")
			return
		}
		sumWS := map[float64]float64{}
		for _, g := range num {
			for _, p := range g.points {
				sumWS[p[0]] += p[1]
			}
		}
		sumCap := map[float64]float64{}
		capTotal := 0.0
		for _, g := range den {
			for _, p := range g.points {
				sumCap[p[0]] += p[1]
			}
		}
		for _, v := range sumCap {
			if v > capTotal {
				capTotal = v // 全集群 capacity 近似恒定，作时间点兜底
			}
		}
		tss := make([]float64, 0, len(sumWS))
		for ts := range sumWS {
			tss = append(tss, ts)
		}
		sort.Float64s(tss)
		pts := make([][2]float64, 0, len(tss))
		for _, ts := range tss {
			c := sumCap[ts]
			if c == 0 {
				c = capTotal
			}
			if c == 0 {
				continue
			}
			pts = append(pts, [2]float64{ts, sumWS[ts] / c * 100})
		}
		if len(pts) == 0 {
			addC("mem", "内存使用率", "%", "", "cluster.memory.utilization")
			return
		}
		res.Series = append(res.Series, MetricSeries{Key: "mem", Name: "内存使用率", Unit: "%", Points: pts})
	}

	addC("cpu", "CPU 使用率", "%", "", "cluster.cpu.utilization")
	addClusterMemPhysical()
	addC("cpu_use", "CPU 使用", "", "使用", "cluster.cpu.usage_rate")
	addC("cpu_req", "CPU 请求", "", "请求", "cluster.cpu.request")
	addC("cpu_lim", "CPU 上限", "", "上限", "cluster.cpu.limit")
	addC("mem_req", "内存请求", "bytes", "请求", "cluster.memory.request")
	addC("mem_lim", "内存上限", "bytes", "上限", "cluster.memory.limit")
	addC("fs_avail", "文件系统可用", "bytes", "可用", "cluster.filesystem.available")
	addC("fs_lim", "文件系统总量", "bytes", "总量", "cluster.filesystem.limit")
	addNode("node_cpu", "节点 CPU 使用率", "%", "node.cpu.utilization")
	// 内存走物理总量口径（working_set / capacity），与 Prometheus 看板一致、≤100%。
	addNodeRatio("node_mem", "节点内存使用率", "%", "node.memory.working_set", "node.memory.capacity", 100)
	// 磁盘：(总量-可用)/总量，与 ARMS 节点磁盘口径一致。
	addNodeUsedRatio("node_disk", "节点磁盘使用率", "node.filesystem.available", "node.filesystem.limit")

	res.Cards = []MetricCard{
		{Key: "cpu", Name: "集群 CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "集群内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "cpu_core", Name: "CPU 核数（使用/请求/上限）", Unit: "", SeriesKeys: []string{"cpu_use", "cpu_req", "cpu_lim"}},
		{Key: "mem_alloc", Name: "内存（请求/上限）", Unit: "bytes", SeriesKeys: []string{"mem_req", "mem_lim"}},
		{Key: "fs", Name: "文件系统（可用/总量）", Unit: "bytes", SeriesKeys: []string{"fs_avail", "fs_lim"}},
		{Key: "node_cpu", Name: "各节点 CPU 使用率", Unit: "%", SeriesKeys: []string{"node_cpu"}},
		{Key: "node_mem", Name: "各节点内存使用率", Unit: "%", SeriesKeys: []string{"node_mem"}},
		{Key: "node_disk", Name: "各节点磁盘使用率", Unit: "%", SeriesKeys: []string{"node_disk"}},
	}
	if len(res.Series) == 0 {
		// 边缘集群走到这里说明 acs_k8s（其首选）无数据 → 再回退 ARMS Prometheus。
		// 非边缘集群的 ARMS 已在上方优先尝试过，此处不重复。
		if edge {
			if base := s.armsPromBase(ds, rid, region); base != "" {
				armsDS := &model.DataSource{Type: model.DataSourcePrometheus, URL: base}
				s.promK8sFill(noProxyClient(15*time.Second), base, armsDS, lookback, step, res)
				if len(res.Series) > 0 {
					res.SourceName = strings.TrimSpace(ds.Name + " · ARMS Prometheus")
					return res, nil
				}
				res.Series = res.Series[:0]
			}
		}
		res.Cards = nil
		res.Message = "由于产品升级迭代，新创建的容器集群在云监控中将默认不再提供 Kubernetes 容器监控能力，建议用户统一使用可观测监控 Prometheus 版提供容器监控，可点「完整监控」到控制台查看。"
	}
	return res, nil
}

// appMetrics 应用服务（K8s 工作负载）监控：从该应用来源的 Prometheus（kube-state-metrics + cadvisor）
// 实时查询副本数（可用/期望）、CPU、内存、Pod 重启。按 namespace + 工作负载名定位，Pod 用正则匹配。
func (s *MetricsService) appMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	ns := firstField(data, "namespace")
	app := firstField(data, "app_name")
	wtype := firstField(data, "workload_type")
	if ns == "" || app == "" {
		res.Message = "该应用缺少命名空间/名称，无法关联监控"
		return res, nil
	}
	res.Target = ns + "/" + app

	// 取查询用的 Prometheus 地址与鉴权：自建=数据源地址；阿里云 ACK=该集群 ARMS Prometheus(token 在 URL、免鉴权)
	base := ""
	qds := ds
	ackDetail := "" // ACK 应用的「完整监控」跳该集群 ACK 控制台监控页（与资源实例里集群的完整监控一致）
	switch ds.Type {
	case model.DataSourcePrometheus:
		base = strings.TrimRight(ds.URL, "/")
	case model.DataSourceAliyun:
		rid, region := s.ackClusterInfo(ds, firstField(data, "cluster_name"))
		base = armsPromBaseURL(ds, rid, region)
		qds = &model.DataSource{Type: model.DataSourcePrometheus} // ARMS 查询地址内嵌 token，不加鉴权头
		if base == "" {
			res.Message = "无法获取该集群的 ARMS Prometheus 地址（集群可能未接入「可观测监控 Prometheus 版」）"
			return res, nil
		}
		if rid != "" {
			ackDetail = fmt.Sprintf("https://cs.console.aliyun.com/#/k8s/cluster/%s/v2/info/overview?tab=monitor&clusterType=ManagedKubernetes&region=%s", rid, region)
		}
	default:
		res.Message = "该应用来源暂不支持监控（" + ds.Type + "）"
		return res, nil
	}
	nsSel := fmt.Sprintf(`namespace="%s"`, ns)

	// 副本可用/期望：按工作负载类型取对应 kube-state-metrics 指标
	var availExpr, desiredExpr string
	switch wtype {
	case "StatefulSet":
		availExpr = fmt.Sprintf(`kube_statefulset_status_replicas_ready{%s,statefulset="%s"}`, nsSel, app)
		desiredExpr = fmt.Sprintf(`kube_statefulset_replicas{%s,statefulset="%s"}`, nsSel, app)
	case "DaemonSet":
		availExpr = fmt.Sprintf(`kube_daemonset_status_number_available{%s,daemonset="%s"}`, nsSel, app)
		desiredExpr = fmt.Sprintf(`kube_daemonset_status_desired_number_scheduled{%s,daemonset="%s"}`, nsSel, app)
	default: // Deployment 及其它
		availExpr = fmt.Sprintf(`kube_deployment_status_replicas_available{%s,deployment="%s"}`, nsSel, app)
		desiredExpr = fmt.Sprintf(`kube_deployment_spec_replicas{%s,deployment="%s"}`, nsSel, app)
	}

	// Pod 正则（匹配该工作负载的 Pod 名；Prometheus 标签匹配整串锚定）
	esc := regexEscape(app)
	var podRe string
	switch wtype {
	case "StatefulSet":
		podRe = esc + "-[0-9]+" // app-0 / app-1（有序，精确避免误配）
	default:
		// Deployment/DaemonSet/其它：宽松前缀匹配。严格「app-<rs>-<pod>」两段正则对多段/超长工作负载名会漏匹配
		podRe = esc + "-.*"
	}
	podSel := fmt.Sprintf("namespace=\"%s\",pod=~`%s`,container!=\"\",container!=\"POD\"", ns, podRe)

	end := time.Now().Unix()
	start := end - int64(lookback)
	client := noProxyClient(12 * time.Second)
	// 工作负载级单线（副本可用/期望）
	add := func(key, name, unit, label, expr string) {
		pts, err := promQueryRange(client, base, qds, expr, start, end, step)
		if err != nil {
			return // 单条失败不致命
		}
		res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: label, Points: pts})
	}
	// 按 Pod 拆分的多线（CPU/内存/重启，各 Pod 一条，不合并）
	addPods := func(key, name, unit, expr string) {
		rows, err := promQueryRangeMulti(client, base, qds, expr, start, end, step, "pod")
		if err != nil {
			return
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].label < rows[j].label })
		for _, r := range rows {
			res.Series = append(res.Series, MetricSeries{Key: key, Name: name, Unit: unit, Label: shortPod(r.label, app), Points: r.points})
		}
	}
	add("rep_avail", "副本数", "个", "可用", availExpr)
	add("rep_desired", "副本数", "个", "期望", desiredExpr)
	addPods("app_cpu", "CPU 使用", "核", fmt.Sprintf("sum by (pod) (rate(container_cpu_usage_seconds_total{%s}[5m]))", podSel))
	addPods("app_mem", "内存使用", "bytes", fmt.Sprintf("sum by (pod) (container_memory_working_set_bytes{%s})", podSel))
	addPods("app_restarts", "Pod 重启", "次", fmt.Sprintf("sum by (pod) (kube_pod_container_status_restarts_total{namespace=\"%s\",pod=~`%s`})", ns, podRe))

	res.Cards = []MetricCard{
		{Key: "replicas", Name: "副本数（可用/期望）", Unit: "个", SeriesKeys: []string{"rep_avail", "rep_desired"}},
		{Key: "cpu", Name: "CPU 使用（核）", Unit: "核", SeriesKeys: []string{"app_cpu"}},
		{Key: "mem", Name: "内存使用", Unit: "bytes", SeriesKeys: []string{"app_mem"}},
		{Key: "restarts", Name: "Pod 重启次数", Unit: "次", SeriesKeys: []string{"app_restarts"}},
	}
	// 「完整监控」：ACK 应用→与资源实例里该集群一致(ACK 控制台监控页)；
	// 否则复用数据源「监控地址」模板(对应集群 Grafana)填充应用占位符；再回退 Prometheus 图形界面
	if ackDetail != "" {
		res.DetailURL = ackDetail
	} else if tpl := strings.TrimSpace(ds.MonitorURL); tpl != "" {
		res.DetailURL = fillMonitorTpl(tpl, map[string]string{
			"namespace": ns, "workload": app, "app": app, "pod": app, "cluster": firstField(data, "cluster_name"),
		})
	} else {
		res.DetailURL = fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=0&g0.range_input=1h", base,
			url.QueryEscape(fmt.Sprintf("sum(rate(container_cpu_usage_seconds_total{%s}[5m]))", podSel)))
	}
	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该应用在 Prometheus 中暂无监控数据"
	}
	return res, nil
}

// shortPod 去掉 Pod 名里的工作负载前缀，只留区分用的后缀（如 app-<rs>-<hash> → <rs>-<hash>），用作多线图例。
func shortPod(pod, app string) string {
	if app != "" && strings.HasPrefix(pod, app+"-") {
		return pod[len(app)+1:]
	}
	return pod
}
