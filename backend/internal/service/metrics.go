package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// MetricsService 主机资源监控取数：按实例来源（Prometheus/阿里云/腾讯云）自动路由到对应监控后端。
// 复用实例同步时的数据源（instance.source_id → data_source）凭证，无需另配。
type MetricsService struct {
	db    *gorm.DB
	inst  *InstanceService
	cache sync.Map // key "instanceID:range" -> *metricsCacheEntry（短 TTL，减轻 Prometheus/云监控压力）
}

// metricsCacheEntry 指标结果缓存项。
type metricsCacheEntry struct {
	at  time.Time
	res *HostMetricsResult
}

// metricsCacheTTL 指标缓存有效期：取数走 Prometheus/ARMS 较慢(0.5~3.6s/集群)，
// 大屏/多人访问与 30s 自动刷新在此窗口内直接命中缓存，避免重复打后端。
const metricsCacheTTL = 30 * time.Second

func NewMetricsService(db *gorm.DB, inst *InstanceService) *MetricsService {
	return &MetricsService{db: db, inst: inst}
}

// MetricSeries 单条指标时序。Points 为 [秒级时间戳, 值] 数组。
// Label 为同一卡片内区分多条线的名称（如磁盘的挂载点、网络的进/出）；单线指标留空。
type MetricSeries struct {
	Key    string       `json:"key"`
	Name   string       `json:"name"`
	Unit   string       `json:"unit"`
	Label  string       `json:"label,omitempty"`
	Points [][2]float64 `json:"points"`
}

// MetricCard 前端一张图卡的定义：Name 标题、Unit 单位（%/bps//s/空=计数），
// SeriesKeys 指明本卡包含哪些 series（按 series.key 匹配；多个=多条线，如网络进出、磁盘各挂载点）。
type MetricCard struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Unit       string   `json:"unit"`
	SeriesKeys []string `json:"series_keys"`
}

// HostMetricsResult 资源监控返回（主机/MySQL 等通用）。Supported=false 时 Message 说明原因。
type HostMetricsResult struct {
	Supported  bool           `json:"supported"`
	SourceType string         `json:"source_type"`
	SourceName string         `json:"source_name"`
	Target     string         `json:"target"` // 实际用于取数的标识（IP / 实例ID）
	Range      string         `json:"range"`
	Step       int            `json:"step"`
	Message    string         `json:"message,omitempty"`
	DetailURL  string         `json:"detail_url,omitempty"` // 「查看完整监控」深链（控制台/Prometheus）
	Cards      []MetricCard   `json:"cards"`                // 前端按此顺序渲染图卡
	Series     []MetricSeries `json:"series"`
}

// hostCards 主机四图卡布局（CPU/内存/磁盘/网络）。
func hostCards() []MetricCard {
	return []MetricCard{
		{Key: "cpu", Name: "CPU 使用率", Unit: "%", SeriesKeys: []string{"cpu"}},
		{Key: "mem", Name: "内存使用率", Unit: "%", SeriesKeys: []string{"mem"}},
		{Key: "disk", Name: "磁盘使用率", Unit: "%", SeriesKeys: []string{"disk"}},
		{Key: "net", Name: "网络带宽", Unit: "bps", SeriesKeys: []string{"net_in", "net_out"}},
	}
}

// objIDOf 取实例所属模型的 obj_id。
func (s *MetricsService) objIDOf(modelID uint) string {
	var m model.Model
	if s.db.Select("obj_id").First(&m, modelID).Error != nil {
		return ""
	}
	return m.ObjID
}

// rangeSpec 把 range 关键字映射为 (回看秒数, 采样步长秒数)。
func rangeSpec(rng string) (lookback, step int) {
	switch rng {
	case "6h":
		return 6 * 3600, 120
	case "24h", "1d":
		return 24 * 3600, 300
	case "7d":
		return 7 * 24 * 3600, 1800
	case "1h":
		fallthrough
	default:
		return 3600, 60
	}
}

// HostMetrics 查询某主机实例近 rng 时段的 CPU/内存/磁盘/网络指标。
// HostMetrics 取实例监控（带短 TTL 缓存）。缓存命中直接返回，未命中则实取并回填。
func (s *MetricsService) HostMetrics(instanceID uint, rng string) (*HostMetricsResult, error) {
	key := strconv.FormatUint(uint64(instanceID), 10) + ":" + rng
	if v, ok := s.cache.Load(key); ok {
		if e, ok := v.(*metricsCacheEntry); ok && time.Since(e.at) < metricsCacheTTL {
			return e.res, nil
		}
	}
	res, err := s.hostMetricsUncached(instanceID, rng)
	if err == nil && res != nil {
		s.cache.Store(key, &metricsCacheEntry{at: time.Now(), res: res})
	}
	return res, err
}

// hostMetricsUncached 实际取数逻辑（无缓存）。
func (s *MetricsService) hostMetricsUncached(instanceID uint, rng string) (*HostMetricsResult, error) {
	var inst model.Instance
	if err := s.db.First(&inst, instanceID).Error; err != nil {
		return nil, ErrNotFound
	}
	lookback, step := rangeSpec(rng)
	res := &HostMetricsResult{Range: rng, Step: step, Series: []MetricSeries{}}

	if inst.SourceID == 0 {
		res.Message = "该实例为手动创建，无关联监控数据源"
		return res, nil
	}
	var ds model.DataSource
	if err := s.db.First(&ds, inst.SourceID).Error; err != nil {
		res.Message = "未找到实例来源数据源"
		return res, nil
	}
	res.SourceType = ds.Type
	res.SourceName = ds.Name

	data := decodeInstData(inst.Data)
	objID := s.objIDOf(inst.ModelID)

	var out *HostMetricsResult
	var err error
	switch objID {
	case "mysql":
		out, err = s.mysqlMetrics(&ds, data, lookback, step, res)
	case "eip":
		out, err = s.eipMetrics(&ds, data, lookback, step, res)
	case "redis":
		out, err = s.redisMetrics(&ds, data, lookback, step, res)
	case "kafka":
		out, err = s.kafkaMetrics(&ds, data, lookback, step, res)
	case "clb":
		out, err = s.clbMetrics(&ds, data, lookback, step, res)
	case "nlb":
		out, err = s.nlbMetrics(&ds, data, lookback, step, res)
	case "alb":
		out, err = s.albMetrics(&ds, data, lookback, step, res)
	case "cen":
		out, err = s.cenMetrics(&ds, data, lookback, step, res)
	case "vbr":
		out, err = s.vbrMetrics(&ds, data, lookback, step, res)
	case "minio":
		out, err = s.minioMetrics(&ds, data, lookback, step, res)
	case "nacos":
		out, err = s.nacosMetrics(&ds, data, lookback, step, res)
	case "lindorm":
		out, err = s.lindormMetrics(&ds, data, lookback, step, res)
	case "elastic":
		out, err = s.elasticMetrics(&ds, data, lookback, step, res)
	case "natgw":
		out, err = s.natgwMetrics(&ds, data, lookback, step, res)
	case "apigw":
		out, err = s.apigMetrics(&ds, data, lookback, step, res)
	case "kubernetes":
		out, err = s.k8sMetrics(&ds, data, lookback, step, res)
	case "k8s_app":
		out, err = s.appMetrics(&ds, data, lookback, step, res)
	default: // host 及其它主机类
		switch ds.Type {
		case model.DataSourcePrometheus:
			out, err = s.promHostMetrics(&ds, data, lookback, step, res)
		case model.DataSourceAliyun:
			out, err = s.aliyunHostMetrics(&ds, data, lookback, step, res)
		case model.DataSourceTencent:
			out, err = s.tencentHostMetrics(&ds, data, lookback, step, res)
		default:
			res.Message = "该来源暂不支持监控（" + ds.Type + "）"
			return res, nil
		}
		if out != nil && out.Supported && len(out.Cards) == 0 {
			out.Cards = hostCards()
		}
	}
	// 数据源配了自定义监控模板（Grafana 等）则覆盖「完整监控」深链；否则保留各适配器的默认（Prometheus/云控制台）。
	// MinIO 例外：该模板是主机 node 看板，对 MinIO 无意义，保留适配器自带的 MinIO 专属看板深链。
	if err == nil && out != nil && objID != "minio" && objID != "kubernetes" && objID != "k8s_app" && strings.TrimSpace(ds.MonitorURL) != "" {
		vars := baseMonitorVars(data)
		// Prometheus 源且模板用到 ${job}：实时从 Prometheus 读该主机真实 job 标签（兼容任意 job 命名）
		if ds.Type == model.DataSourcePrometheus && strings.Contains(ds.MonitorURL, "${job}") {
			vars["job"] = s.promHostJob(&ds, instIP(data))
		}
		out.DetailURL = fillMonitorTpl(ds.MonitorURL, vars)
	}
	if out != nil {
		pruneEmptyCards(out)
	}
	return out, err
}

// pruneEmptyCards 剔除没有任何对应 series 的图卡（如云监控没返回的指标）。
func pruneEmptyCards(res *HostMetricsResult) {
	has := map[string]bool{}
	for _, s := range res.Series {
		has[s.Key] = true
	}
	kept := res.Cards[:0]
	for _, c := range res.Cards {
		ok := false
		for _, k := range c.SeriesKeys {
			if has[k] {
				ok = true
				break
			}
		}
		if ok {
			kept = append(kept, c)
		}
	}
	res.Cards = kept
}

// baseMonitorVars 由主机字段构造监控模板变量：ip/subnet/host/id/region。
func baseMonitorVars(data map[string]interface{}) map[string]string {
	ip := instIP(data)
	return map[string]string{
		"ip":     ip,
		"subnet": ipv4Slash24(ip),
		"host":   firstField(data, "bk_host_name", "inst_name", "host_name"),
		"id":     firstField(data, "resource_id", "instance_id", "bk_cloud_inst_id"),
		"region": firstField(data, "bk_zone", "region", "bk_region"),
	}
}

// fillMonitorTpl 用 vars 替换监控模板占位符 ${key}。值做 URL 转义。
func fillMonitorTpl(tpl string, vars map[string]string) string {
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "${"+k+"}", url.QueryEscape(v))
	}
	return tpl
}

// ipv4Slash24 返回 IPv4 的 /24 网段，如 172.21.32.108 → 172.21.32.0/24；非法则空。
func ipv4Slash24(ip string) string {
	p := net.ParseIP(strings.TrimSpace(ip))
	if p == nil || p.To4() == nil {
		return ""
	}
	parts := strings.Split(p.To4().String(), ".")
	if len(parts) != 4 {
		return ""
	}
	return parts[0] + "." + parts[1] + "." + parts[2] + ".0/24"
}

// decodeInstData 解析实例 data JSON。
func decodeInstData(raw []byte) map[string]interface{} {
	m := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// instIP 取主机内网 IP（匹配 Prometheus node_exporter 的 instance 标签）。
func instIP(data map[string]interface{}) string {
	for _, f := range ipFields {
		if v, ok := data[f]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
				// 可能是 "ip1,ip2" 取第一个
				if i := strings.IndexAny(s, ",;"); i > 0 {
					s = s[:i]
				}
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// —— Prometheus (node_exporter) 适配 ——

// promHostMetrics 用 node_exporter 指标画 CPU/内存/磁盘/网络。
func (s *MetricsService) promHostMetrics(ds *model.DataSource, data map[string]interface{}, lookback, step int, res *HostMetricsResult) (*HostMetricsResult, error) {
	ip := instIP(data)
	if ip == "" {
		res.Message = "该主机无内网 IP，无法关联监控"
		return res, nil
	}
	res.Target = ip
	// instance 标签通常是裸 IP，也兼容 IP:port。
	// 用 PromQL 反引号原始字符串包裹正则（双引号里 \. 会被判为非法转义）。
	sel := fmt.Sprintf("instance=~`%s(:[0-9]+)?`", regexEscape(ip))
	netDev := `,device!~"lo|veth.*|docker.*|br-.*|cali.*|flannel.*|cni.*|tunl.*|kube.*"`
	fsExcl := `,fstype!~"tmpfs|overlay|squashfs|iso9660|ramfs|devtmpfs"`

	// 单线指标：cpu/mem/net_in/net_out
	queries := []struct {
		key, name, unit, expr string
	}{
		{"cpu", "CPU 使用率", "%",
			fmt.Sprintf(`100 - (avg(rate(node_cpu_seconds_total{%s,mode="idle"}[5m])) * 100)`, sel)},
		{"mem", "内存使用率", "%",
			fmt.Sprintf(`(1 - node_memory_MemAvailable_bytes{%s} / node_memory_MemTotal_bytes{%s}) * 100`, sel, sel)},
		// 网络统一为 bit/s（×8），与云监控口径一致
		{"net_in", "网络入流量", "bps",
			fmt.Sprintf(`sum(rate(node_network_receive_bytes_total{%s%s}[5m])) * 8`, sel, netDev)},
		{"net_out", "网络出流量", "bps",
			fmt.Sprintf(`sum(rate(node_network_transmit_bytes_total{%s%s}[5m])) * 8`, sel, netDev)},
	}

	end := time.Now().Unix()
	start := end - int64(lookback)
	client := noProxyClient(12 * time.Second)
	base := strings.TrimRight(ds.URL, "/")
	for _, q := range queries {
		pts, err := promQueryRange(client, base, ds, q.expr, start, end, step)
		if err != nil {
			// 单条失败不致命，继续其它指标
			continue
		}
		res.Series = append(res.Series, MetricSeries{Key: q.key, Name: q.name, Unit: q.unit, Points: pts})
	}

	// 磁盘：按挂载点返回多条线（不做 max 聚合），label=挂载点。
	diskExpr := fmt.Sprintf(`100 - (node_filesystem_avail_bytes{%s%s} / node_filesystem_size_bytes{%s%s} * 100)`, sel, fsExcl, sel, fsExcl)
	if disks, err := promQueryRangeMulti(client, base, ds, diskExpr, start, end, step, "mountpoint"); err == nil {
		for _, d := range disks {
			res.Series = append(res.Series, MetricSeries{Key: "disk", Name: "磁盘使用率", Unit: "%", Label: d.label, Points: d.points})
		}
	}

	// 「查看完整监控」深链到 Prometheus 图形界面（CPU 表达式）
	cpuExpr := fmt.Sprintf(`100 - (avg(rate(node_cpu_seconds_total{%s,mode="idle"}[5m])) * 100)`, sel)
	res.DetailURL = fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=0&g0.range_input=1h", base, url.QueryEscape(cpuExpr))

	res.Supported = true
	if len(res.Series) == 0 {
		res.Message = "该主机在 Prometheus 中暂无监控数据"
	}
	return res, nil
}

// promHostJob 实时查询某主机在 Prometheus 中的 job 标签（兼容任意 job 命名，不依赖 /24 规律）。
func (s *MetricsService) promHostJob(ds *model.DataSource, ip string) string {
	if ip == "" || ds.URL == "" {
		return ""
	}
	sel := fmt.Sprintf("instance=~`%s(:[0-9]+)?`", regexEscape(ip))
	expr := fmt.Sprintf("node_boot_time_seconds{%s}", sel)
	form := url.Values{}
	form.Set("query", expr)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(ds.URL, "/")+"/api/v1/query", strings.NewReader(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if ds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ds.Token)
	} else if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}
	resp, err := noProxyClient(8 * time.Second).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Data.Result) == 0 {
		return ""
	}
	return out.Data.Result[0].Metric["job"]
}

// promQueryRange 调用 Prometheus /api/v1/query_range，返回首条 series 的 [ts,val] 点集。
func promQueryRange(client *http.Client, base string, ds *model.DataSource, expr string, start, end int64, step int) ([][2]float64, error) {
	form := url.Values{}
	form.Set("query", expr)
	form.Set("start", strconv.FormatInt(start, 10))
	form.Set("end", strconv.FormatInt(end, 10))
	form.Set("step", strconv.Itoa(step))
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/query_range", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if ds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ds.Token)
	} else if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][2]interface{} `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status != "success" || len(out.Data.Result) == 0 {
		return [][2]float64{}, nil
	}
	raw := out.Data.Result[0].Values
	pts := make([][2]float64, 0, len(raw))
	for _, v := range raw {
		if len(v) != 2 {
			continue
		}
		ts, _ := toFloat(v[0])
		val, ok := toFloat(v[1])
		if !ok {
			continue // NaN / 字符串非数值跳过
		}
		pts = append(pts, [2]float64{ts, val})
	}
	return pts, nil
}

// labeledSeries query_range 多 series 结果：label 取自指定标签，points 为点集。
type labeledSeries struct {
	label  string
	points [][2]float64
}

// promQueryRangeMulti 返回全部 series，并按 labelKey 命名（如 mountpoint）。label 为空时用序号兜底。
func promQueryRangeMulti(client *http.Client, base string, ds *model.DataSource, expr string, start, end int64, step int, labelKey string) ([]labeledSeries, error) {
	form := url.Values{}
	form.Set("query", expr)
	form.Set("start", strconv.FormatInt(start, 10))
	form.Set("end", strconv.FormatInt(end, 10))
	form.Set("step", strconv.Itoa(step))
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/query_range", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if ds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ds.Token)
	} else if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]interface{}  `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		return []labeledSeries{}, nil
	}
	list := make([]labeledSeries, 0, len(out.Data.Result))
	for i, r := range out.Data.Result {
		label := r.Metric[labelKey]
		if label == "" {
			label = fmt.Sprintf("#%d", i+1)
		}
		pts := make([][2]float64, 0, len(r.Values))
		for _, v := range r.Values {
			if len(v) != 2 {
				continue
			}
			ts, _ := toFloat(v[0])
			val, ok := toFloat(v[1])
			if !ok {
				continue
			}
			pts = append(pts, [2]float64{ts, val})
		}
		list = append(list, labeledSeries{label: label, points: pts})
	}
	// 按挂载点名排序，保证线条顺序稳定（/ 在前）
	sort.Slice(list, func(i, j int) bool { return list[i].label < list[j].label })
	return list, nil
}

// noProxyClient 构造不走 HTTP 代理的客户端：监控后端多为内网地址，
// 若走环境里的 http_proxy 会连不通。
func noProxyClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:               nil, // 忽略 HTTP_PROXY/HTTPS_PROXY
		DialContext:         (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 8 * time.Second,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// regexEscape 转义 PromQL 正则里的点等特殊字符。
func regexEscape(s string) string {
	r := strings.NewReplacer(".", `\.`, "+", `\+`, "*", `\*`, "?", `\?`, "(", `\(`, ")", `\)`, "[", `\[`, "]", `\]`)
	return r.Replace(s)
}
