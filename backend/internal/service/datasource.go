package service

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// DataSourceService 外部数据源配置与同步。
type DataSourceService struct {
	db   *gorm.DB
	inst *InstanceService
}

func NewDataSourceService(db *gorm.DB, inst *InstanceService) *DataSourceService {
	return &DataSourceService{db: db, inst: inst}
}

func (s *DataSourceService) List() ([]model.DataSource, error) {
	var list []model.DataSource
	err := s.db.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

// Reorder 按给定 id 顺序重排数据源（sort_order 从 1 递增）。
func (s *DataSourceService) Reorder(orderedIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range orderedIDs {
			if err := tx.Model(&model.DataSource{}).
				Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DataSourceOption 数据源精简项（不含凭证，供导出等场景选择来源）。
type DataSourceOption struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ListOptions 返回数据源精简列表（id/name/type），不含 AK/SK 等敏感字段。
func (s *DataSourceService) ListOptions() ([]DataSourceOption, error) {
	var out []DataSourceOption
	err := s.db.Model(&model.DataSource{}).Select("id, name, type").Order("id asc").Scan(&out).Error
	return out, err
}

func (s *DataSourceService) Create(d *model.DataSource) error {
	if d.Type == "" {
		d.Type = model.DataSourcePrometheus
	}
	if strings.TrimSpace(d.Name) == "" {
		return newValidationErr("名称必填")
	}
	if d.Type == model.DataSourceAliyun {
		if d.Username == "" || d.Password == "" {
			return newValidationErr("阿里云需填 AccessKey 与 AccessSecret")
		}
	} else if d.Type == model.DataSourceTencent {
		if d.Username == "" || d.Password == "" {
			return newValidationErr("腾讯云需填 SecretId 与 SecretKey")
		}
	} else if strings.TrimSpace(d.URL) == "" {
		return newValidationErr("地址必填")
	}
	return s.db.Create(d).Error
}

func (s *DataSourceService) Update(id uint, d *model.DataSource) error {
	var existing model.DataSource
	if err := s.db.First(&existing, id).Error; err != nil {
		return ErrNotFound
	}
	existing.Name = d.Name
	existing.URL = d.URL
	existing.Query = d.Query
	existing.Region = d.Region
	existing.OnlyIP = d.OnlyIP
	existing.MonitorURL = d.MonitorURL
	existing.Cron = d.Cron
	existing.Enabled = d.Enabled
	// 密钥类字段留空表示“不修改”，避免误清空
	if d.Username != "" {
		existing.Username = d.Username
	}
	if d.Password != "" {
		existing.Password = d.Password
	}
	if d.Token != "" {
		existing.Token = d.Token
	}
	return s.db.Save(&existing).Error
}

func (s *DataSourceService) Delete(id uint) error {
	return s.db.Delete(&model.DataSource{}, id).Error
}

// TestConnection 测试数据源连通性（不落库），返回查询到的条数。
func (s *DataSourceService) TestConnection(d *model.DataSource) (int, error) {
	if d.Type == model.DataSourceAliyun {
		return s.aliTestConnection(d)
	}
	if d.Type == model.DataSourceTencent {
		return s.tcTestConnection(d)
	}
	if strings.TrimSpace(d.URL) == "" {
		return 0, newValidationErr("地址必填")
	}
	q := strings.TrimSpace(d.Query)
	if q == "" {
		q = "up"
	}
	series, err := promQuery(d.URL, d.Token, d.Username, d.Password, q)
	if err != nil {
		return 0, err
	}
	return len(series), nil
}

// Sync 按数据源类型分发同步（统一入口），结果写入 last_sync。
func (s *DataSourceService) Sync(id uint) (*SyncReport, error) {
	var ds model.DataSource
	if err := s.db.First(&ds, id).Error; err != nil {
		return nil, ErrNotFound
	}
	if !ds.Enabled {
		return nil, newValidationErr("数据源已禁用，请先启用再同步")
	}
	switch ds.Type {
	case model.DataSourceAliyun:
		report, err := s.SyncAliyun(&ds)
		if err != nil {
			s.writeLastSync(id, "失败："+err.Error())
			return nil, err
		}
		s.writeLastSync(id, syncMsg(report))
		return report, nil
	case model.DataSourceTencent:
		report, err := s.SyncTencent(&ds)
		if err != nil {
			s.writeLastSync(id, "失败："+err.Error())
			return nil, err
		}
		s.writeLastSync(id, syncMsg(report))
		return report, nil
	default:
		return s.SyncPrometheus(id, "")
	}
}

func syncMsg(r *SyncReport) string {
	return "发现" + strconv.Itoa(r.Discovered) + " 新增" + strconv.Itoa(r.Created) +
		" 更新" + strconv.Itoa(r.Updated) + " 跳过" + strconv.Itoa(r.Skipped) + " 失败" + strconv.Itoa(r.Failed)
}

// SyncReport 同步结果。
type SyncReport struct {
	Discovered int      `json:"discovered"` // 发现的主机数
	Created    int      `json:"created"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"` // 因非 IP 被跳过的目标数
	Failed     int      `json:"failed"`
	Errors     []string `json:"errors"`

	// skippedModels 记录本次因产品级错误（无权限/未开通/接口失败）被整体跳过的模型 obj_id，
	// 这些模型本次未能读到数据，不参与失联下线标记（避免把实际在线资源误判为下线）。
	skippedModels map[string]bool
}

// skipProduct 记录某产品（模型 obj_id）本次因错误被跳过：追加一条可读错误并标记，供 MarkOffline 规避。
func (r *SyncReport) skipProduct(objID, msg string) {
	if len(r.Errors) < 100 {
		r.Errors = append(r.Errors, msg)
	}
	if r.skippedModels == nil {
		r.skippedModels = map[string]bool{}
	}
	r.skippedModels[objID] = true
}

// CreatorSync 自动同步/采集来源的统一创建人标识（用于区分手动创建）。
const CreatorSync = "prometheus"

// SyncPrometheus 从 Prometheus 查询并按内网IP upsert 主机。
func (s *DataSourceService) SyncPrometheus(id uint, user string) (*SyncReport, error) {
	var ds model.DataSource
	if err := s.db.First(&ds, id).Error; err != nil {
		return nil, ErrNotFound
	}
	if ds.Type != model.DataSourcePrometheus {
		return nil, newValidationErr("非 Prometheus 数据源")
	}
	if !ds.Enabled {
		return nil, newValidationErr("数据源已禁用，请先启用再同步")
	}
	query := strings.TrimSpace(ds.Query)
	if query == "" {
		query = "up"
	}
	series, err := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, query)
	if err != nil {
		s.writeLastSync(id, "失败："+err.Error())
		return nil, newValidationErr("查询 Prometheus 失败：%s", err.Error())
	}

	// 额外查 node_uname_info（主机名/内核）与 node_os_info（发行版）补全信息（best effort）
	uname := map[string]map[string]string{}
	if us, e := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, "node_uname_info"); e == nil {
		for _, m := range us {
			if ip := hostFromInstance(m["instance"]); ip != "" {
				uname[ip] = m
			}
		}
	}
	osInfo := map[string]string{} // ip -> 操作系统发行版名
	if os, e := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, "node_os_info"); e == nil {
		for _, m := range os {
			ip := hostFromInstance(m["instance"])
			if ip == "" {
				continue
			}
			if v := firstNonEmpty(m["pretty_name"], strings.TrimSpace(m["name"]+" "+m["version"])); v != "" {
				osInfo[ip] = v
			}
		}
	}
	// CPU核数 / 内存(MB) / 磁盘(GB)（node_exporter 指标，best effort）
	numByIP := func(q string) map[string]float64 {
		if m, e := promQueryValues(ds.URL, ds.Token, ds.Username, ds.Password, q); e == nil {
			return m
		}
		return map[string]float64{}
	}
	cpu := numByIP(`count by (instance) (node_cpu_seconds_total{mode="idle"})`)
	mem := numByIP(`node_memory_MemTotal_bytes/1024/1024`)
	disk := numByIP(`sum by (instance) (node_filesystem_size_bytes{fstype!~"tmpfs|devtmpfs|overlay|squashfs|iso9660|ramfs|nsfs"})/1024/1024/1024`)

	// 存活状态：up 指标值 1/0
	upVal := numByIP("up")
	// 状态枚举选项 id（按名称关键词解析，默认 Running=1 / Down=2）
	runID, downID := "1", "2"
	var stAttr model.Attribute
	if s.db.Where("model_id = ? AND property_id = ?", s.inst.HostModelID(), "bk_host_status").First(&stAttr).Error == nil {
		var opts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(stAttr.Options, &opts)
		for _, o := range opts {
			n := strings.ToLower(o.Name)
			if strings.Contains(n, "run") || strings.Contains(o.Name, "运行") || strings.Contains(o.Name, "在线") {
				runID = o.ID
			}
			if strings.Contains(n, "down") || strings.Contains(o.Name, "宕") || strings.Contains(o.Name, "离线") {
				downID = o.ID
			}
		}
	}

	// 云厂商枚举：标签值(名称或id) -> 选项id
	cpMap := map[string]string{}
	var cpAttr model.Attribute
	if s.db.Where("model_id = ? AND property_id = ?", s.inst.HostModelID(), "cloud_provider").First(&cpAttr).Error == nil {
		var opts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(cpAttr.Options, &opts)
		for _, o := range opts {
			cpMap[o.ID] = o.ID
			cpMap[o.Name] = o.ID
			cpMap[strings.ToLower(o.ID)] = o.ID
			cpMap[strings.ToLower(o.Name)] = o.ID
		}
	}

	report := &SyncReport{Errors: []string{}}
	seen := map[string]bool{}
	seenIDs := []uint{}
	for _, metric := range series {
		ip := hostFromInstance(metric["instance"])
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		if ds.OnlyIP && !isIPv4(ip) {
			report.Skipped++
			continue
		}
		report.Discovered++

		u := uname[ip] // 可能为 nil
		data := map[string]interface{}{"bk_host_innerip": ip}
		name := firstNonEmpty(metric["nodename"], metric["node_name"], metric["hostname"], u["nodename"], u["node_name"], u["hostname"])
		if name != "" {
			data["bk_host_name"] = name
		}
		// 操作系统：优先发行版名(node_os_info)，否则回退内核(sysname+release)
		os := osInfo[ip]
		if os == "" {
			os = buildOS(metric)
		}
		if os == "" {
			os = buildOS(u)
		}
		if os != "" {
			data["bk_os_name"] = os
		}
		if v, ok := cpu[ip]; ok {
			data["bk_cpu"] = float64(int64(v + 0.5))
		}
		if v, ok := mem[ip]; ok {
			data["bk_mem"] = float64(int64(v + 0.5))
		}
		if v, ok := disk[ip]; ok {
			data["bk_disk"] = float64(int64(v + 0.5))
		}
		// 地域（region 标签 → 文本字段 bk_zone）
		if region := firstNonEmpty(metric["region"], u["region"]); region != "" {
			data["bk_zone"] = region
		}
		// 云厂商（cloud_provider 标签名称/id → 枚举选项id）
		if cp := firstNonEmpty(metric["cloud_provider"], u["cloud_provider"]); cp != "" {
			if id := firstNonEmpty(cpMap[cp], cpMap[strings.ToLower(cp)]); id != "" {
				data["cloud_provider"] = id
			}
		}
		// 状态（up 1→Running / 0→Down）
		if v, ok := upVal[ip]; ok {
			if v >= 0.5 {
				data["bk_host_status"] = runID
			} else {
				data["bk_host_status"] = downID
			}
		}
		inst, created, e := s.inst.CollectHost(data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, ip+": "+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	// 失联标记：本次未出现的（该数据源同步的主机）置为「已下线」
	_, _ = s.inst.MarkOffline(s.inst.HostModelID(), ds.ID, seenIDs)

	// 附加：从同一 Prometheus 同步自建 MySQL（mysqld_exporter），best-effort、不影响主机结果
	s.syncPromMySQL(ds, report)
	// 附加：从 redis_exporter 同步自建 Redis，best-effort
	s.syncPromRedis(ds, report)
	// 附加：从 MinIO 原生指标同步自建 MinIO 集群，best-effort
	s.syncPromMinio(ds, report)
	// 附加：从 kube-state-metrics 同步 Kubernetes 集群级记录，best-effort
	s.syncPromK8s(ds, report)
	// 附加：从 kube-state-metrics 同步 K8s 工作负载到「应用服务」模型，best-effort
	s.syncPromK8sApps(ds, report)

	s.writeLastSync(id, fmt.Sprintf("发现%d 新增%d 更新%d 跳过%d 失败%d",
		report.Discovered, report.Created, report.Updated, report.Skipped, report.Failed))
	return report, nil
}

// syncPromMySQL 从 mysqld_exporter 指标同步自建 MySQL 到「MySQL」模型。
//
// 约定（在 Prometheus 抓取目标上打标签，会随 mysql_up 带出）：
//
//	inst_name   实例名（唯一键；缺省用 endpoint:port 兜底）
//	endpoint    数据库地址（缺省从 instance 标签拆出 exporter 主机）
//	port        数据库端口（缺省 3306）
//	resource_id 实例ID（可选，想和云同步对齐时填一致）
//	region / arch / spec             地域 / 实例类型 / 规格（文本标签）
//	cpu_cores / mem_mb / disk_gb     数值标签
//
// version 来自 mysql_version_info，status 来自 mysql_up(1→运行中/0→异常)。
// 仅写标签给出的字段（缺省不覆盖），因此不会把已有值冲空。
func (s *DataSourceService) syncPromMySQL(ds model.DataSource, report *SyncReport) {
	modelID := s.inst.ModelIDByObjID("mysql")
	if modelID == 0 {
		return
	}
	ups, err := promQueryLV(ds.URL, ds.Token, ds.Username, ds.Password, "mysql_up")
	if err != nil {
		if len(report.Errors) < 100 {
			report.Errors = append(report.Errors, "MySQL(mysql_up) 同步跳过："+err.Error())
		}
		return
	}
	if len(ups) == 0 {
		return // 该 Prometheus 无 mysqld_exporter 指标：静默跳过，避免误标下线
	}
	// 云厂商枚举：标签值(名称或 id，大小写不敏感) → 选项 id
	cpMap := map[string]string{}
	var cpAttr model.Attribute
	if s.db.Where("model_id = ? AND property_id = ?", modelID, "cloud_provider").First(&cpAttr).Error == nil {
		var opts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(cpAttr.Options, &opts)
		for _, o := range opts {
			cpMap[strings.ToLower(o.ID)] = o.ID
			cpMap[strings.ToLower(o.Name)] = o.ID
		}
	}
	// 版本：mysql_version_info{version=...}，按 instance 标签对齐
	verByInst := map[string]string{}
	if vs, e := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, "mysql_version_info"); e == nil {
		for _, m := range vs {
			if v := firstNonEmpty(m["version"], m["innodb_version"]); v != "" {
				verByInst[m["instance"]] = v
			}
		}
	}

	seenIDs := []uint{}
	seen := map[string]bool{}
	for _, se := range ups {
		m := se.labels
		instLabel := m["instance"]
		endpoint := firstNonEmpty(m["endpoint"], hostFromInstance(instLabel))
		port := strings.TrimSpace(m["port"])
		instName := firstNonEmpty(m["inst_name"], joinHostPort(endpoint, port), instLabel)
		if instName == "" || seen[instName] {
			continue
		}
		seen[instName] = true
		report.Discovered++

		data := map[string]interface{}{"inst_name": instName}
		if endpoint != "" {
			data["endpoint"] = endpoint
		}
		if n, ok := atoiOK(port); ok {
			data["port"] = n
		}
		if v := verByInst[instLabel]; v != "" {
			data["version"] = v
		}
		if rid := strings.TrimSpace(m["resource_id"]); rid != "" && rid != "-" && !strings.EqualFold(rid, "n/a") {
			data["resource_id"] = rid
		}
		if v := strings.TrimSpace(m["region"]); v != "" {
			data["region"] = v
		}
		if v := strings.TrimSpace(m["arch"]); v != "" {
			data["arch"] = v
		}
		if v := strings.TrimSpace(m["spec"]); v != "" {
			data["spec"] = v
		}
		if n, ok := atoiOK(m["cpu_cores"]); ok {
			data["cpu_cores"] = n
		}
		if n, ok := atoiOK(m["mem_mb"]); ok {
			data["mem_mb"] = n
		}
		if n, ok := atoiOK(m["disk_gb"]); ok {
			data["disk_gb"] = n
		}
		if cp := strings.TrimSpace(m["cloud_provider"]); cp != "" {
			if id := cpMap[strings.ToLower(cp)]; id != "" {
				data["cloud_provider"] = id
			}
		}
		if se.value >= 0.5 {
			data["status"] = "running"
		} else {
			data["status"] = "abnormal"
		}

		inst, created, e := s.inst.CollectCloudInstance(modelID, "inst_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "MySQL "+instName+"："+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	// 本次未出现的（该数据源同步的 MySQL）置为「已下线」
	_, _ = s.inst.MarkOffline(modelID, ds.ID, seenIDs)
}

// syncPromMinio 从 MinIO 原生 Prometheus 指标同步自建 MinIO 集群到「MinIO」模型。
//
// MinIO 的指标是集群级，按 instance 标签（MinIO 服务地址）聚合一条：
//
//	minio_software_version_info{version, inst_name}   版本（server 端为发布时间戳）
//	minio_cluster_capacity_usable_total_bytes         可用总容量
//	minio_cluster_capacity_usable_free_bytes          可用剩余容量 → 已用 = 总-剩
//	minio_cluster_health_status                       健康(1→运行中 / 0→异常)
//
// 唯一键用 instance(IP)。同一集群可能被重复采集(带/不带 inst_name 标签各一份)，按 IP 聚合去重。
func (s *DataSourceService) syncPromMinio(ds model.DataSource, report *SyncReport) {
	modelID := s.inst.ModelIDByObjID("minio")
	if modelID == 0 {
		return
	}
	vers, err := promQueryLV(ds.URL, ds.Token, ds.Username, ds.Password, "minio_software_version_info")
	if err != nil {
		if len(report.Errors) < 100 {
			report.Errors = append(report.Errors, "MinIO(minio_software_version_info) 同步跳过："+err.Error())
		}
		return
	}
	if len(vers) == 0 {
		return // 无 MinIO 指标：静默跳过
	}
	totalGB := promNumByHost(s, ds, "minio_cluster_capacity_usable_total_bytes/1024/1024/1024")
	freeGB := promNumByHost(s, ds, "minio_cluster_capacity_usable_free_bytes/1024/1024/1024")
	health := promNumByHost(s, ds, "minio_cluster_health_status")

	// 按 instance(IP) 聚合：版本、实例名及可选标签（取带标签的那份）
	type agg struct{ version, instName, console, endpoint, region string }
	clusters := map[string]*agg{}
	setIf := func(dst *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = v
		}
	}
	for _, se := range vers {
		ip := hostFromInstance(se.labels["instance"])
		if ip == "" {
			continue
		}
		c := clusters[ip]
		if c == nil {
			c = &agg{}
			clusters[ip] = c
		}
		setIf(&c.version, se.labels["version"])
		setIf(&c.instName, se.labels["inst_name"])
		// 可选业务标签（在 Prometheus 抓取目标上打）：控制台地址 / 访问地址 / 地域
		setIf(&c.console, se.labels["console"])
		setIf(&c.endpoint, firstNonEmpty(se.labels["endpoint"], se.labels["address"]))
		setIf(&c.region, se.labels["region"])
	}

	seenIDs := []uint{}
	for ip, c := range clusters {
		instName := firstNonEmpty(c.instName, ip)
		report.Discovered++
		data := map[string]interface{}{
			"inst_name":      instName,
			"endpoint":       firstNonEmpty(c.endpoint, ip),
			"resource_id":    ip,
			"cloud_provider": "idc",
		}
		if c.version != "" {
			data["version"] = c.version
		}
		if c.console != "" {
			data["console"] = c.console
		}
		if c.region != "" {
			data["region"] = c.region
		}
		if t, ok := totalGB[ip]; ok {
			if f, ok2 := freeGB[ip]; ok2 && t >= f {
				data["used_gb"] = float64(int64(t - f + 0.5))
			}
		}
		if h, ok := health[ip]; ok && h < 0.5 {
			data["status"] = "abnormal"
		} else {
			data["status"] = "running"
		}
		inst, created, e := s.inst.CollectCloudInstance(modelID, "inst_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "MinIO "+instName+"："+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	_, _ = s.inst.MarkOffline(modelID, ds.ID, seenIDs)
}

// promNumByHost 执行 PromQL 并返回 host(IP) → 数值，查询失败返回空 map。
func promNumByHost(s *DataSourceService, ds model.DataSource, query string) map[string]float64 {
	if m, e := promQueryValues(ds.URL, ds.Token, ds.Username, ds.Password, query); e == nil {
		return m
	}
	return map[string]float64{}
}

// syncPromRedis 从 redis_exporter 指标同步自建 Redis 到「Redis」模型。
//
// 以 redis_up 为在线列表，redis_instance_info 补全版本/模式/端口（按 instance 标签对齐）：
//
//	redis_up{instance}                                在线状态（1→运行中 / 0→异常）
//	redis_instance_info{redis_version, redis_mode, tcp_port, role, ...}
//	redis_memory_max_bytes（可选）                     最大内存 → mem_gb
//
// 唯一键用 endpoint:port（跨重启稳定）。仅写有值字段，不覆盖已有值；无 redis_up 则静默跳过。
func (s *DataSourceService) syncPromRedis(ds model.DataSource, report *SyncReport) {
	modelID := s.inst.ModelIDByObjID("redis")
	if modelID == 0 {
		return
	}
	ups, err := promQueryLV(ds.URL, ds.Token, ds.Username, ds.Password, "redis_up")
	if err != nil {
		if len(report.Errors) < 100 {
			report.Errors = append(report.Errors, "Redis(redis_up) 同步跳过："+err.Error())
		}
		return
	}
	if len(ups) == 0 {
		return // 无 redis_exporter 指标：静默跳过
	}
	// 版本/模式/端口：redis_instance_info，按 instance 标签对齐
	info := map[string]map[string]string{}
	if is, e := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, "redis_instance_info"); e == nil {
		for _, m := range is {
			info[m["instance"]] = m
		}
	}
	// 最大内存（可选）：instance(IP) → GB
	maxMemGB := map[string]float64{}
	if m, e := promQueryValues(ds.URL, ds.Token, ds.Username, ds.Password, "redis_memory_max_bytes/1024/1024/1024"); e == nil {
		maxMemGB = m
	}

	seenIDs := []uint{}
	seen := map[string]bool{}
	for _, se := range ups {
		instLabel := se.labels["instance"]
		meta := info[instLabel] // 可能为 nil
		endpoint := hostFromInstance(instLabel)
		port := firstNonEmpty(meta["tcp_port"], portFromInstance(instLabel))
		instName := firstNonEmpty(se.labels["inst_name"], meta["inst_name"], joinHostPort(endpoint, port), instLabel)
		if instName == "" || seen[instName] {
			continue
		}
		seen[instName] = true
		report.Discovered++

		data := map[string]interface{}{"inst_name": instName}
		if endpoint != "" {
			data["endpoint"] = endpoint
		}
		if n, ok := atoiOK(port); ok {
			data["port"] = n
		}
		// 稳定唯一键：endpoint:port
		if rid := joinHostPort(endpoint, port); rid != "" {
			data["resource_id"] = rid
		}
		if v := strings.TrimSpace(meta["redis_version"]); v != "" {
			data["version"] = v
		}
		// 部署模式 → 架构枚举（standalone→标准版 / cluster→集群版）
		switch strings.ToLower(strings.TrimSpace(meta["redis_mode"])) {
		case "cluster":
			data["arch"] = "cluster"
		case "standalone", "sentinel":
			data["arch"] = "standard"
		}
		if g, ok := maxMemGB[endpoint]; ok && g >= 1 {
			data["mem_gb"] = float64(int64(g + 0.5))
		}
		if se.value >= 0.5 {
			data["status"] = "running"
		} else {
			data["status"] = "abnormal"
		}

		inst, created, e := s.inst.CollectCloudInstance(modelID, "inst_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "Redis "+instName+"："+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	// 本次未出现的（该数据源同步的 Redis）置为「已下线」
	_, _ = s.inst.MarkOffline(modelID, ds.ID, seenIDs)
}

// syncPromK8s 从 kube-state-metrics 指标同步 Kubernetes 集群级记录到「Kubernetes」模型。
//
// 以 kube_node_info 聚合：按 cluster 标签分组（无 cluster 标签时整份视作一个集群，集群名用数据源名），
// 节点数=去重节点数，版本=出现最多的 kubelet_version；集群存活状态按节点就绪情况判定。
// Prometheus 侧无法获取 API Server / Pod 网段 / CNI / 集群ID，这些字段留空不覆盖。
// 查不到 kube_node_info 时静默跳过，避免把已有集群误标下线。
func (s *DataSourceService) syncPromK8s(ds model.DataSource, report *SyncReport) {
	modelID := s.inst.ModelIDByObjID("kubernetes")
	if modelID == 0 {
		return
	}
	nodes, err := promQuery(ds.URL, ds.Token, ds.Username, ds.Password, "kube_node_info")
	if err != nil {
		if len(report.Errors) < 100 {
			report.Errors = append(report.Errors, "Kubernetes(kube_node_info) 同步跳过："+err.Error())
		}
		return
	}
	if len(nodes) == 0 {
		return // 无 kube-state-metrics 指标：静默跳过
	}

	// 就绪节点集合：kube_node_status_condition{condition="Ready",status="true"} > 0
	readyNodes := map[string]bool{}
	if rs, e := promQueryLV(ds.URL, ds.Token, ds.Username, ds.Password,
		`kube_node_status_condition{condition="Ready",status="true"}`); e == nil {
		for _, se := range rs {
			if se.value >= 0.5 {
				if n := firstNonEmpty(se.labels["node"], se.labels["node_name"]); n != "" {
					readyNodes[clusterNodeKey(se.labels["cluster"], n)] = true
				}
			}
		}
	}

	type clusterAgg struct {
		nodeSet  map[string]bool
		verCount map[string]int
		readyCnt int
	}
	clusters := map[string]*clusterAgg{}
	for _, m := range nodes {
		node := firstNonEmpty(m["node"], m["node_name"], m["instance"])
		if node == "" {
			continue
		}
		key := strings.TrimSpace(m["cluster"]) // 可能为空
		c := clusters[key]
		if c == nil {
			c = &clusterAgg{nodeSet: map[string]bool{}, verCount: map[string]int{}}
			clusters[key] = c
		}
		if !c.nodeSet[node] {
			c.nodeSet[node] = true
			if readyNodes[clusterNodeKey(key, node)] {
				c.readyCnt++
			}
		}
		if v := firstNonEmpty(m["kubelet_version"], m["kubeletVersion"]); v != "" {
			c.verCount[v]++
		}
	}

	seenIDs := []uint{}
	for key, c := range clusters {
		name := key
		if name == "" {
			name = ds.Name // 单集群：用数据源名作集群名
		}
		report.Discovered++
		data := map[string]interface{}{
			"cluster_name":   name,
			"cloud_provider": "idc",
			"node_count":     float64(len(c.nodeSet)),
		}
		if v := mostCommon(c.verCount); v != "" {
			data["version"] = strings.TrimPrefix(v, "v")
		}
		// 状态：有就绪节点→运行中；有节点但无就绪→异常
		if len(readyNodes) > 0 && c.readyCnt == 0 {
			data["status"] = "abnormal"
		} else {
			data["status"] = "running"
		}
		inst, created, e := s.inst.CollectCloudInstance(modelID, "cluster_name", data, ds.Name, ds.ID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "Kubernetes "+name+"："+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
	}
	// 本次未出现的（该数据源同步的 Kubernetes 集群）置为「已下线」
	_, _ = s.inst.MarkOffline(modelID, ds.ID, seenIDs)
}

// syncPromK8sApps 从 kube-state-metrics 同步 K8s 工作负载到「应用服务」(k8s_app) 模型。best-effort，不影响主机结果。
// 覆盖 Deployment / StatefulSet / DaemonSet：
//
//	期望副本 kube_deployment_spec_replicas / kube_statefulset_replicas / kube_daemonset_status_desired_number_scheduled
//	可用副本 kube_deployment_status_replicas_available / kube_statefulset_status_replicas_ready / kube_daemonset_status_number_available
//
// 标签 namespace + (deployment|statefulset|daemonset) + cluster（单集群 Prometheus 常无 cluster 标签→用数据源名）。
// app_key=cluster/ns/type/name 作唯一键；镜像经 kube_pod_owner+kube_pod_container_info best-effort 关联；
// 并按 cluster_name 自动建 belong_k8sapp_kubernetes 关联到同源集群实例。
// syncK8sApps 从任一 Prometheus(base，kube-state-metrics)枚举 K8s 工作负载→k8s_app。
// forceCluster 非空时强制用它作 cluster_name(ACK：ARMS Prometheus 单集群、无 cluster 标签)；
// 为空时用指标 cluster 标签，再回退 creator(数据源名)。creator=写入的创建人(数据源名)。
// 下线标记：forceCluster 非空时只下线本集群本次未出现的(多集群共用一个阿里云源，不能整源下线)。
func (s *DataSourceService) syncK8sApps(base, token, user, pass, forceCluster, creator string, sourceID uint, report *SyncReport) {
	modelID := s.inst.ModelIDByObjID("k8s_app")
	if modelID == 0 {
		return
	}
	type wl struct {
		cluster, namespace, name, wtype string
		desired, available              float64
		hasAvail                        bool
	}
	keyOf := func(cluster, ns, wtype, name string) string {
		return strings.TrimSpace(cluster) + "/" + ns + "/" + wtype + "/" + name
	}
	wls := map[string]*wl{}
	addDesired := func(query, wtype, nameLabel string) {
		rs, e := promQueryLV(base, token, user, pass, query)
		if e != nil {
			return
		}
		for _, se := range rs {
			name := se.labels[nameLabel]
			if name == "" {
				continue
			}
			k := keyOf(se.labels["cluster"], se.labels["namespace"], wtype, name)
			w := wls[k]
			if w == nil {
				w = &wl{cluster: strings.TrimSpace(se.labels["cluster"]), namespace: se.labels["namespace"], name: name, wtype: wtype}
				wls[k] = w
			}
			w.desired = se.value
		}
	}
	addAvail := func(query, wtype, nameLabel string) {
		rs, e := promQueryLV(base, token, user, pass, query)
		if e != nil {
			return
		}
		for _, se := range rs {
			name := se.labels[nameLabel]
			if name == "" {
				continue
			}
			if w := wls[keyOf(se.labels["cluster"], se.labels["namespace"], wtype, name)]; w != nil {
				w.available = se.value
				w.hasAvail = true
			}
		}
	}
	addDesired("kube_deployment_spec_replicas", "Deployment", "deployment")
	addDesired("kube_statefulset_replicas", "StatefulSet", "statefulset")
	addDesired("kube_daemonset_status_desired_number_scheduled", "DaemonSet", "daemonset")
	if len(wls) == 0 {
		return // 无 kube-state-metrics 工作负载指标：静默跳过
	}
	addAvail("kube_deployment_status_replicas_available", "Deployment", "deployment")
	addAvail("kube_statefulset_status_replicas_ready", "StatefulSet", "statefulset")
	addAvail("kube_daemonset_status_number_available", "DaemonSet", "daemonset")

	images := k8sWorkloadImagesAt(base, token, user, pass) // key(cluster/ns/type/name) -> 主容器镜像
	// Fallback：部分集群的 kube-state-metrics 不暴露 kube_pod_owner(如 IOT-K8s-01)，
	// 上面按属主关联会全空。此时用已知工作负载名直接匹配 kube_pod_container_info 的 Pod 名补齐镜像。
	{
		var refs []wlRef
		for k, w := range wls {
			if images[k] == "" {
				refs = append(refs, wlRef{ns: w.namespace, wtype: w.wtype, name: w.name, key: k})
			}
		}
		if len(refs) > 0 {
			imagesFromPodNames(base, token, user, pass, refs, images)
		}
	}
	assocID := s.inst.AssocIDByCode("belong_k8sapp_kubernetes")
	k8sModelID := s.inst.ModelIDByObjID("kubernetes")
	clusterInst := map[string]uint{}
	// 应用业务线跟随所属集群：按 cluster_name+source 取集群实例的 biz_line（懒加载缓存）
	clusterBiz := map[string]string{}
	bizOfCluster := func(cluster string) string {
		if v, ok := clusterBiz[cluster]; ok {
			return v
		}
		v := ""
		if k8sModelID != 0 {
			var ci model.Instance
			if s.db.Where("model_id = ? AND source_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name')) = ?",
				k8sModelID, sourceID, cluster).First(&ci).Error == nil {
				var cd map[string]interface{}
				if json.Unmarshal(ci.Data, &cd) == nil {
					if b, ok := cd["biz_line"].(string); ok {
						v = b
					}
				}
			}
		}
		clusterBiz[cluster] = v
		return v
	}

	seenIDs := []uint{}
	for key, w := range wls {
		cluster := forceCluster
		if cluster == "" {
			cluster = w.cluster
			if cluster == "" {
				cluster = creator // 单集群 Prometheus 无 cluster 标签：用数据源名
			}
		}
		// forceCluster 下 app_key 用强制集群名，保证跨轮询唯一键稳定
		appKey := key
		if forceCluster != "" {
			appKey = keyOf(forceCluster, w.namespace, w.wtype, w.name)
		}
		report.Discovered++
		data := map[string]interface{}{
			"app_key":          appKey,
			"app_name":         w.name,
			"cluster_name":     cluster,
			"namespace":        w.namespace,
			"workload_type":    w.wtype,
			"replicas_desired": w.desired,
			"status":           appWorkloadStatus(w.desired, w.available, w.hasAvail),
			"cloud_provider":   "idc",
		}
		if w.hasAvail {
			data["replicas_available"] = w.available
		}
		if img := images[key]; img != "" {
			data["image"] = img
		}
		if b := bizOfCluster(cluster); b != "" {
			data["biz_line"] = b // 业务线与所属集群保持一致
		}
		inst, created, e := s.inst.CollectCloudInstance(modelID, "app_key", data, creator, sourceID)
		if e != nil {
			report.Failed++
			if len(report.Errors) < 100 {
				report.Errors = append(report.Errors, "应用服务 "+w.name+"："+e.Error())
			}
			continue
		}
		seenIDs = append(seenIDs, inst.ID)
		if created {
			report.Created++
		} else {
			report.Updated++
		}
		if assocID != 0 && k8sModelID != 0 {
			cid, ok := clusterInst[cluster]
			if !ok {
				cid = s.inst.FindInstanceIDByProp(k8sModelID, "cluster_name", cluster, sourceID)
				clusterInst[cluster] = cid
			}
			if cid != 0 {
				_ = s.inst.EnsureRelation(assocID, inst.ID, cid)
			}
		}
	}
	// 下线：forceCluster 下只限本集群（阿里云源多集群共用），否则整源
	if forceCluster != "" {
		q := s.db.Model(&model.Instance{}).
			Where("model_id = ? AND source_id = ?", modelID, sourceID).
			Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name')) = ?", forceCluster).
			Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.status')) <> ?", model.OfflineStatusID)
		if len(seenIDs) > 0 {
			q = q.Where("id NOT IN ?", seenIDs)
		}
		q.UpdateColumn("data", gorm.Expr("JSON_SET(data, '$.status', ?)", model.OfflineStatusID))
	} else {
		_, _ = s.inst.MarkOffline(modelID, sourceID, seenIDs)
	}
}

// syncPromK8sApps 自建 Prometheus 数据源同步 K8s 应用（保持原入口）。
func (s *DataSourceService) syncPromK8sApps(ds model.DataSource, report *SyncReport) {
	s.syncK8sApps(ds.URL, ds.Token, ds.Username, ds.Password, "", ds.Name, ds.ID, report)
}

// SyncAckApps 同步某个阿里云 ACK 集群的应用服务（走该集群的 ARMS Prometheus / kube-state-metrics）。
func (s *DataSourceService) SyncAckApps(dsID uint, clusterName string) (*SyncReport, error) {
	var ds model.DataSource
	if s.db.First(&ds, dsID).Error != nil {
		return nil, ErrNotFound
	}
	if ds.Type != model.DataSourceAliyun {
		return nil, newValidationErr("仅阿里云数据源支持 ACK 应用同步")
	}
	k8sModelID := s.inst.ModelIDByObjID("kubernetes")
	var inst model.Instance
	if s.db.Where("model_id = ? AND source_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name')) = ?",
		k8sModelID, dsID, clusterName).First(&inst).Error != nil {
		return nil, newValidationErr("未找到该集群（请先同步 ACK 集群，或确认集群名/来源）")
	}
	var d map[string]interface{}
	_ = json.Unmarshal(inst.Data, &d)
	clusterID, _ := d["resource_id"].(string)
	region, _ := d["region"].(string)
	if strings.TrimSpace(clusterID) == "" {
		return nil, newValidationErr("该集群缺少集群ID(resource_id)")
	}
	base := armsPromBaseURL(&ds, clusterID, region)
	if base == "" {
		return nil, newValidationErr("无法获取该集群的 ARMS Prometheus 地址（集群可能未接入「可观测监控 Prometheus 版」）")
	}
	report := &SyncReport{Errors: []string{}}
	s.syncK8sApps(base, "", "", "", clusterName, ds.Name, dsID, report)
	return report, nil
}

// appWorkloadStatus 由期望/可用副本推导应用状态。
func appWorkloadStatus(desired, available float64, hasAvail bool) string {
	if desired <= 0 {
		return "stopped"
	}
	if !hasAvail {
		return "running" // 无可用副本指标：有期望副本即视为运行中
	}
	if available <= 0 {
		return "abnormal"
	}
	if available < desired {
		return "degraded" // 部分可用
	}
	return "running"
}

// k8sWorkloadImages best-effort 关联「工作负载 → 主容器镜像」。
// kube_pod_owner{namespace,pod,owner_kind,owner_name} + kube_pod_container_info{namespace,pod,container,image}。
// Deployment 的 Pod 属主是 ReplicaSet(名=deploy-<hash>)，剥掉末段 hash 还原 Deployment 名；
// StatefulSet/DaemonSet 的 owner_name 直接是工作负载名。镜像取任一非 pause 容器。
func k8sWorkloadImagesAt(base, token, user, pass string) map[string]string {
	out := map[string]string{}
	owners, e1 := promQuery(base, token, user, pass, "kube_pod_owner")
	infos, e2 := promQuery(base, token, user, pass, "kube_pod_container_info")
	if e1 != nil || e2 != nil || len(owners) == 0 || len(infos) == 0 {
		return out
	}
	podImg := map[string]string{} // cluster/ns/pod -> image
	for _, m := range infos {
		pod, img := m["pod"], m["image"]
		if pod == "" || img == "" || strings.Contains(img, "pause") {
			continue
		}
		pk := strings.TrimSpace(m["cluster"]) + "/" + m["namespace"] + "/" + pod
		if _, ok := podImg[pk]; !ok {
			podImg[pk] = img
		}
	}
	for _, m := range owners {
		pod, kind, owner := m["pod"], m["owner_kind"], m["owner_name"]
		if pod == "" || owner == "" {
			continue
		}
		cluster := strings.TrimSpace(m["cluster"])
		img := podImg[cluster+"/"+m["namespace"]+"/"+pod]
		if img == "" {
			continue
		}
		var wtype, name string
		switch kind {
		case "ReplicaSet":
			wtype, name = "Deployment", trimRSHash(owner)
		case "StatefulSet", "DaemonSet":
			wtype, name = kind, owner
		default:
			continue
		}
		k := cluster + "/" + m["namespace"] + "/" + wtype + "/" + name
		if _, ok := out[k]; !ok {
			out[k] = img
		}
	}
	return out
}

// wlRef 是工作负载的轻量描述（命名空间/类型/名称/唯一键），供 Pod 名匹配补镜像用。
type wlRef struct{ ns, wtype, name, key string }

// imagesFromPodNames 当 kube_pod_owner 缺失时的兜底：用已知工作负载名按 Pod 命名规则匹配
// kube_pod_container_info 的 Pod，补齐 images[key]。只填 images 里尚为空的键，不覆盖已有值。
// 命名规则：Deployment/DaemonSet 为 <name>-<随机段>（宽松后缀，兼容多段/超长工作负载名），
// StatefulSet 为 <name>-<序号>。同命名空间内按名称长度降序匹配，避免前缀碰撞（app vs app-worker）。
func imagesFromPodNames(base, token, user, pass string, refs []wlRef, images map[string]string) {
	infos, err := promQuery(base, token, user, pass, "kube_pod_container_info")
	if err != nil || len(infos) == 0 {
		return
	}
	// 每个 (ns,pod) 取首个非 pause 容器镜像
	type podKey struct{ ns, pod string }
	podImg := map[podKey]string{}
	for _, m := range infos {
		ns, pod, img := m["namespace"], m["pod"], m["image"]
		if pod == "" || img == "" || strings.Contains(img, "pause") {
			continue
		}
		pk := podKey{ns, pod}
		if _, ok := podImg[pk]; !ok {
			podImg[pk] = img
		}
	}
	// 按命名空间分组，名称长者优先
	type wre struct {
		key  string
		name string
		re   *regexp.Regexp
	}
	byNS := map[string][]wre{}
	for _, r := range refs {
		suf := "-.*" // Deployment/DaemonSet：宽松后缀
		if r.wtype == "StatefulSet" {
			suf = "-[0-9]+"
		}
		re, e := regexp.Compile("^" + regexp.QuoteMeta(r.name) + suf + "$")
		if e != nil {
			continue
		}
		byNS[r.ns] = append(byNS[r.ns], wre{r.key, r.name, re})
	}
	for ns := range byNS {
		g := byNS[ns]
		sort.SliceStable(g, func(i, j int) bool { return len(g[i].name) > len(g[j].name) })
		byNS[ns] = g
	}
	for pk, img := range podImg {
		for _, wr := range byNS[pk.ns] {
			if images[wr.key] == "" && wr.re.MatchString(pk.pod) {
				images[wr.key] = img
				break
			}
		}
	}
}

// trimRSHash 去掉 ReplicaSet 名末尾的 "-<hash>" 段还原 Deployment 名。
func trimRSHash(rs string) string {
	if i := strings.LastIndex(rs, "-"); i > 0 {
		return rs[:i]
	}
	return rs
}

// clusterNodeKey 组合 cluster + node 作为节点唯一键（跨集群同名节点不冲突）。
func clusterNodeKey(cluster, node string) string {
	return strings.TrimSpace(cluster) + "/" + node
}

// mostCommon 返回计数 map 中出现次数最多的键（并列时取字典序较大者，偏向较新版本）。
func mostCommon(counts map[string]int) string {
	best, bestN := "", -1
	for k, n := range counts {
		if n > bestN || (n == bestN && k > best) {
			best, bestN = k, n
		}
	}
	return best
}

// joinHostPort 拼 host:port（port 为空只返回 host；host 为空返回空串）。
func joinHostPort(host, port string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if port = strings.TrimSpace(port); port != "" {
		return host + ":" + port
	}
	return host
}

// atoiOK 解析非负整数标签为 float64（数值字段校验要求 float64），非法/空返回 ok=false。
func atoiOK(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return float64(n), true
}

// writeLastSync 记录上次同步时间与结果。
func (s *DataSourceService) writeLastSync(id uint, msg string) {
	if len(msg) > 250 {
		msg = msg[:250]
	}
	s.db.Model(&model.DataSource{}).Where("id = ?", id).
		Updates(map[string]interface{}{"last_sync_at": time.Now(), "last_sync_msg": msg})
}

// promQuery 调用 Prometheus /api/v1/query，返回每条 series 的 metric 标签。
func promQuery(base, token, username, password, query string) ([]map[string]string, error) {
	endpoint := strings.TrimRight(base, "/") + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// 鉴权：优先 Bearer Token，否则账号密码 Basic Auth
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if username != "" {
		req.SetBasicAuth(username, password)
	}
	// 内网数据源直连，不走环境 HTTP 代理
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, newValidationErr("认证失败（HTTP %d），请检查账号密码或 Token", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, newValidationErr("Prometheus 返回 HTTP %d", resp.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		return nil, newValidationErr("Prometheus 返回异常")
	}
	series := make([]map[string]string, 0, len(out.Data.Result))
	for _, r := range out.Data.Result {
		series = append(series, r.Metric)
	}
	return series, nil
}

// promQueryValues 执行 PromQL，返回 instance(IP) -> 数值。
func promQueryValues(base, token, username, password, query string) (map[string]float64, error) {
	endpoint := strings.TrimRight(base, "/") + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if username != "" {
		req.SetBasicAuth(username, password)
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	res := map[string]float64{}
	for _, r := range out.Data.Result {
		ip := hostFromInstance(r.Metric["instance"])
		if ip == "" || len(r.Value) < 2 {
			continue
		}
		if sv, ok := r.Value[1].(string); ok {
			if f, e := strconv.ParseFloat(sv, 64); e == nil {
				res[ip] = f
			}
		}
	}
	return res, nil
}

// promSeriesLV 一条时序：标签 + 数值（用于既要读标签又要读值的场景，如 mysql_up）。
type promSeriesLV struct {
	labels map[string]string
	value  float64
}

// promQueryLV 执行 PromQL，返回每条 series 的标签与数值。
func promQueryLV(base, token, username, password, query string) ([]promSeriesLV, error) {
	endpoint := strings.TrimRight(base, "/") + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if username != "" {
		req.SetBasicAuth(username, password)
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []interface{}     `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	res := make([]promSeriesLV, 0, len(out.Data.Result))
	for _, r := range out.Data.Result {
		v := 0.0
		if len(r.Value) >= 2 {
			if sv, ok := r.Value[1].(string); ok {
				if f, e := strconv.ParseFloat(sv, 64); e == nil {
					v = f
				}
			}
		}
		res = append(res, promSeriesLV{labels: r.Metric, value: v})
	}
	return res, nil
}

// hostFromInstance 从 instance 标签提取主机/IP。
// 兼容 10.0.0.1:9100 / 10.0.0.1 / https://host:443/path / host:443 等形式。
// portFromInstance 从 instance 标签（host:port 或 scheme://host:port/...）提取端口，无则空。
func portFromInstance(instance string) string {
	instance = strings.TrimSpace(instance)
	if i := strings.Index(instance, "://"); i >= 0 {
		instance = instance[i+3:]
	}
	if i := strings.IndexByte(instance, '/'); i >= 0 {
		instance = instance[:i]
	}
	if i := strings.LastIndexByte(instance, ':'); i > 0 {
		if port := instance[i+1:]; port != "" && isAllDigits(port) {
			return port
		}
	}
	return ""
}

func hostFromInstance(instance string) string {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return ""
	}
	// 去掉 scheme://
	if i := strings.Index(instance, "://"); i >= 0 {
		instance = instance[i+3:]
	}
	// 去掉路径
	if i := strings.IndexByte(instance, '/'); i >= 0 {
		instance = instance[:i]
	}
	// host:port -> host（仅当端口为纯数字时去掉）
	if i := strings.LastIndexByte(instance, ':'); i > 0 {
		if port := instance[i+1:]; port != "" && isAllDigits(port) {
			instance = instance[:i]
		}
	}
	return strings.TrimSpace(instance)
}

// isIPv4 判断是否为合法 IPv4 地址。
func isIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 || !isAllDigits(p) {
			return false
		}
		n := 0
		for _, r := range p {
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// buildOS 从 node_uname_info 风格标签拼操作系统描述。
func buildOS(m map[string]string) string {
	parts := []string{}
	if v := m["sysname"]; v != "" {
		parts = append(parts, v)
	}
	if v := m["release"]; v != "" {
		parts = append(parts, v)
	}
	return strings.Join(parts, " ")
}
