package service

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// parseAlertDimensions 把告警维度解析成可读串 "key=value · key=value"（排除冗余的实例ID维度）。
// 兼容三种返回：[]{Key,Value} 数组、JSON 字符串、map。
func parseAlertDimensions(v interface{}) string {
	skip := func(k string) bool {
		lk := strings.ToLower(k)
		return lk == "instanceid" || lk == "instance_id" || lk == "userid"
	}
	add := func(parts *[]string, k, val string) {
		if k != "" && !skip(k) {
			*parts = append(*parts, k+"="+val)
		}
	}
	var parts []string
	switch t := v.(type) {
	case []interface{}: // [{Key,Value}, ...]
		for _, it := range t {
			if m, ok := it.(map[string]interface{}); ok {
				add(&parts, optStr(m["Key"]), optStr(m["Value"]))
			}
		}
	case map[string]interface{}:
		for k, val := range t {
			add(&parts, k, optStr(val))
		}
		sort.Strings(parts)
	case string:
		if t == "" {
			return ""
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(t), &m) == nil {
			for k, val := range m {
				add(&parts, k, optStr(val))
			}
			sort.Strings(parts)
		} else {
			return t
		}
	}
	return strings.Join(parts, " · ")
}

// defaultAlertWindowMin 每次拉取覆盖的默认时间窗（分钟，数据源未配时用）；接口要求 <=15 天。
const defaultAlertWindowMin = 20

// SyncAlerts 拉取某数据源的告警（目前仅阿里云云监控）。返回本次写入/更新条数。
func (s *AlertService) SyncAlerts(id uint) (int, error) {
	var ds model.AlertSource
	if err := s.db.First(&ds, id).Error; err != nil {
		return 0, ErrNotFound
	}
	if !ds.Enabled {
		return 0, newValidationErr("告警源已禁用，请先启用")
	}
	var n int
	var err error
	switch ds.Type {
	case model.AlertSourceAliyun:
		n, err = s.syncAliyunAlerts(&ds)
	case model.AlertSourceArms:
		n, err = s.syncArmsAlerts(&ds)
	case model.AlertSourceTencent:
		n, err = s.syncTencentAlerts(&ds)
	case model.AlertSourcePrometheus:
		n, err = s.syncPrometheusAlerts(&ds)
	default:
		return 0, newValidationErr("仅阿里云 / ARMS / 腾讯云 / Prometheus 告警源支持主动拉取（回调源由平台推送）")
	}
	if err != nil {
		s.writeLastAlert(id, "失败："+err.Error())
		return 0, err
	}
	s.writeLastAlert(id, fmt.Sprintf("成功，处理 %d 条告警", n))
	return n, nil
}

// alertWindowStart 按数据源配置的窗口（分钟，0=默认20，上限15天）算起始时间。
func alertWindowStart(ds *model.AlertSource, now time.Time) time.Time {
	w := ds.AlertWindowMin
	if w <= 0 {
		w = defaultAlertWindowMin
	}
	if w > 15*24*60 {
		w = 15 * 24 * 60
	}
	return now.Add(-time.Duration(w) * time.Minute)
}

func (s *AlertService) writeLastAlert(id uint, msg string) {
	if len(msg) > 250 {
		msg = msg[:250]
	}
	s.db.Model(&model.AlertSource{}).Where("id = ?", id).
		Updates(map[string]interface{}{"last_alert_at": time.Now(), "last_alert_msg": msg})
}

// alertLevelID 阿里云报警级别(P2/P3/P4/OK) → 统一级别 id。
func alertLevelID(l string) string {
	switch strings.ToUpper(strings.TrimSpace(l)) {
	case "P2", "CRITICAL":
		return "critical"
	case "P3", "WARN", "WARNING":
		return "warning"
	case "P4", "INFO":
		return "info"
	case "OK":
		return "ok"
	default:
		return "info"
	}
}

// syncAliyunAlerts 调用云监控 DescribeAlertLogList(2019-01-01) 拉取最近告警历史。
// endpoint: metrics.<region>.aliyuncs.com（CMS 告警为账号级，取数据源配置的首个地域，默认 cn-hangzhou）。
func (s *AlertService) syncAliyunAlerts(ds *model.AlertSource) (int, error) {
	region := firstAlertRegion(ds.Region)
	endpoint := "https://metrics." + region + ".aliyuncs.com"
	now := time.Now()
	start := alertWindowStart(ds, now)

	total := 0
	collected := []*model.Alert{}
	for page := 1; page <= 50; page++ {
		extra := map[string]string{
			"StartTime":  strconv.FormatInt(start.UnixMilli(), 10),
			"EndTime":    strconv.FormatInt(now.UnixMilli(), 10),
			"PageNumber": strconv.Itoa(page),
			"PageSize":   "100",
		}
		raw, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-01-01", "DescribeAlertLogList", extra)
		if err != nil {
			return total, err
		}
		// 阿里云返回里字段类型不固定（string/number），整体解析成 map 再按需取值
		var top map[string]interface{}
		if err := json.Unmarshal(raw, &top); err != nil {
			return total, err
		}
		if code := optStr(top["Code"]); code != "" && code != "200" {
			return total, newValidationErr("阿里云接口错误：%s", optStr(top["Message"]))
		}
		listRaw, _ := top["AlertLogList"].([]interface{})
		if len(listRaw) == 0 {
			break
		}
		for _, item := range listRaw {
			a, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			logID := optStr(a["LogId"])
			if logID == "" {
				continue
			}
			// 跳过被屏蔽(黑名单/沉默)导致「不发送」的日志，不记录
			if desc := aliyunExtended(a, "sendStatusDescription"); desc == "不发送" {
				continue
			}
			if en := aliyunExtended(a, "sendStatusDescriptionEn"); en == "NsNotSend" {
				continue
			}
			// 阿里云 LogId 形如 "<alertId>[seq]@<ts>[n]"，同一次告警每分钟发一条、alertId 相同。
			// 用 alertId 作为去重键，使一次告警的多条分钟日志+恢复日志归并到一行。
			logID = aliyunAlertID(logID)
			level := alertLevelID(optStr(a["Level"]))
			status := "firing"
			at := now
			if ms, e := strconv.ParseInt(optStr(a["AlertTime"]), 10, 64); e == nil && ms > 0 {
				at = time.UnixMilli(ms)
			}
			var resolvedAt *time.Time
			if level == "ok" {
				status = "resolved"
				rt := at
				resolvedAt = &rt
			}
			expr := ""
			if esc, ok := a["Escalation"].(map[string]interface{}); ok {
				expr = optStr(esc["Expression"])
			}
			dims := parseAlertDimensions(a["Dimensions"])
			if dims == "" {
				dims = parseAlertDimensions(a["Instance"]) // 部分接口维度放在 Instance 字段
			}
			al := model.Alert{
				SourceID:     ds.ID,
				LogID:        logID,
				Level:        level,
				Status:       status,
				RuleName:     optStr(a["RuleName"]),
				Product:      optStr(a["Product"]),
				MetricName:   optStr(a["MetricName"]),
				InstanceID:   optStr(a["InstanceId"]),
				InstanceName: optStr(a["InstanceName"]),
				Dimensions:   dims,
				Expression:   expr,
				GroupName:    optStr(a["GroupName"]),
				AlertTime:    at,
				ResolvedAt:   resolvedAt,
			}
			collected = append(collected, &al)
			total++
		}
		if len(listRaw) < 100 {
			break
		}
	}
	// 先触发后恢复逐条写入：恢复日志(level=ok)就地更新对应触发行的恢复时间，不新增数据
	s.upsertAlertsDedup(collected)
	return total, nil
}

// reconcileResolvedAlerts 恢复对账：某条触发告警若存在更晚的 OK 记录（同数据源+资源+规则），
// 标记为已恢复。阿里云的触发/恢复是两条不同日志，否则触发行会一直停在 firing、角标不消。
// sourceID=0 处理全部数据源。
func (s *AlertService) reconcileResolvedAlerts(sourceID uint) {
	// 更晚的恢复记录有两种形态：阿里云单独一条 level='ok' 日志；夜莺/webhook 则是 is_recovered→status='resolved'
	// 的另一条事件（不同 log_id）。两者都用来把仍停在 firing 的触发行标记为已恢复。
	// 把恢复行的时间(优先其 resolved_at，否则 alert_time)写回触发行的 resolved_at，让触发行同时有触发+恢复时间
	sql := `UPDATE alert f
		JOIN alert o ON o.source_id = f.source_id AND o.instance_id = f.instance_id
			AND o.rule_name = f.rule_name AND o.id <> f.id
			AND (o.level = 'ok' OR o.status = 'resolved') AND o.alert_time >= f.alert_time
		SET f.status = 'resolved', f.resolved_at = COALESCE(f.resolved_at, o.resolved_at, o.alert_time)
		WHERE f.status = 'firing' AND f.rule_name <> ''`
	if sourceID > 0 {
		s.db.Exec(sql+" AND f.source_id = ?", sourceID)
	} else {
		s.db.Exec(sql)
	}
}

// ReconcileAllAlerts 对所有数据源做一次恢复对账（启动时调用一次，清理历史卡在 firing 的触发告警）。
func (s *AlertService) ReconcileAllAlerts() { s.reconcileResolvedAlerts(0) }

// ipFields 主机类资源常见的 IP 字段（夜莺/Prometheus 以 IP 作为主机标识时命中）。
var ipFields = []string{"bk_host_innerip", "inner_ip", "innerip", "ip", "private_ip", "host_ip", "bk_host_outerip"}

// resolveAlertResource 把告警关联到 CMDB 实例。多候选标识 + 多字段匹配，覆盖云/夜莺各类资源：
//   - 候选标识：InstanceID、InstanceName，及各自去端口形式（如 1.2.3.4:9100 → 1.2.3.4）；
//   - 匹配字段：IP 候选先匹配常见 IP 字段（主机）；再按 resource_id/instance_id + 各模型唯一字段精确匹配
//     （覆盖 MySQL/Redis/Kafka 等按实例ID或名称标识的资源）。
func (s *AlertService) resolveAlertResource(al *model.Alert) {
	// 0a) k8s 应用告警：带 namespace + (workload|pod) 时，优先关联到 k8s_app 实例
	if al.Namespace != "" && (al.Workload != "" || al.Pod != "") {
		if inst := s.matchK8sApp(al.Cluster, al.Namespace, al.Workload, al.Pod); inst != nil {
			al.ResInstID, al.ResModel = inst.ID, s.inst.modelName(inst.ModelID)
			return
		}
	}
	// 0b) k8s 集群告警：带集群标识(cluster/origin_prometheus)时，关联到 kubernetes 集群实例(cluster_name)
	if al.Cluster != "" {
		if inst := s.matchInstanceByFields(al.Cluster, []string{"cluster_name"}); inst != nil {
			al.ResInstID, al.ResModel = inst.ID, s.inst.modelName(inst.ModelID)
			return
		}
	}
	cands := identCandidates(al)
	if len(cands) == 0 {
		return
	}
	// 1) IP 候选 → 主机 IP 字段
	for _, c := range cands {
		if net.ParseIP(c) == nil {
			continue
		}
		if inst := s.matchInstanceByFields(c, ipFields); inst != nil {
			al.ResInstID, al.ResModel = inst.ID, s.inst.modelName(inst.ModelID)
			return
		}
	}
	// 2) 通用标识字段 + 各模型唯一字段精确匹配
	props := s.identProps()
	for _, c := range cands {
		if inst := s.matchInstanceByFields(c, props); inst != nil {
			al.ResInstID, al.ResModel = inst.ID, s.inst.modelName(inst.ModelID)
			return
		}
	}
}

// matchK8sApp 按 namespace(+可选 cluster) 找 k8s_app 实例：有 workload 则按应用名精确匹配；
// 否则按 pod 名的最长「应用名-」前缀匹配（如 pod=app-<rs>-<hash> → 应用 app）。
func (s *AlertService) matchK8sApp(cluster, namespace, workload, pod string) *model.Instance {
	modelID := s.inst.ModelIDByObjID("k8s_app")
	if modelID == 0 || strings.TrimSpace(namespace) == "" {
		return nil
	}
	q := s.db.Where("model_id = ?", modelID).
		Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.namespace')) = ?", namespace)
	if strings.TrimSpace(cluster) != "" {
		q = q.Where("JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name')) = ?", cluster)
	}
	var insts []model.Instance
	if q.Find(&insts).Error != nil || len(insts) == 0 {
		return nil
	}
	appOf := func(in model.Instance) string {
		var d map[string]interface{}
		if json.Unmarshal(in.Data, &d) == nil {
			if v, ok := d["app_name"].(string); ok {
				return v
			}
		}
		return ""
	}
	if workload != "" {
		for i := range insts {
			if appOf(insts[i]) == workload {
				return &insts[i]
			}
		}
	}
	if pod != "" {
		best := -1
		var bestInst *model.Instance
		for i := range insts {
			name := appOf(insts[i])
			if name != "" && strings.HasPrefix(pod, name+"-") && len(name) > best {
				best = len(name)
				bestInst = &insts[i]
			}
		}
		return bestInst
	}
	return nil
}

// identCandidates 收集告警可用于关联的标识候选（去空去重，含去端口形式）。
func identCandidates(al *model.Alert) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	add(al.InstanceID)
	add(al.InstanceName)
	add(stripPort(al.InstanceID))
	add(stripPort(al.InstanceName))
	return out
}

// identProps 返回用于关联匹配的标识字段：固定的 resource_id/instance_id + 各模型启用的唯一字段。
func (s *AlertService) identProps() []string {
	var uniq []string
	s.db.Model(&model.Attribute{}).Where("`unique` = ? AND enabled = ?", true, true).
		Distinct().Pluck("property_id", &uniq)
	seen := map[string]bool{}
	out := []string{}
	for _, p := range append([]string{"resource_id", "instance_id"}, uniq...) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// matchInstanceByFields 在实例表里按 val = 任一指定 JSON 字段 精确匹配，返回 id 最小的一条。
func (s *AlertService) matchInstanceByFields(val string, fields []string) *model.Instance {
	if strings.TrimSpace(val) == "" || len(fields) == 0 {
		return nil
	}
	conds := make([]string, 0, len(fields))
	args := make([]interface{}, 0, len(fields)*2)
	for _, f := range fields {
		conds = append(conds, "JSON_UNQUOTE(JSON_EXTRACT(data, ?)) = ?")
		args = append(args, "$."+f, val)
	}
	var inst model.Instance
	if s.db.Model(&model.Instance{}).Where(strings.Join(conds, " OR "), args...).
		Order("id asc").First(&inst).Error == nil {
		return &inst
	}
	return nil
}

// AlertListParams 告警列表筛选。
type AlertListParams struct {
	Level     string
	Status    string
	Keyword   string
	SourceID  uint
	ResInstID uint // 按关联的 CMDB 实例筛选（实例详情用）
	Days      int
	Page      int
	PageSize  int
	Sort      string // activity=按最近动静(触发/恢复较晚者)倒序；空=按触发时间倒序
}

// AlertListResult 告警列表结果。
type AlertListResult struct {
	List     []model.Alert `json:"list"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// ListAlerts 分页查询告警，支持级别/状态/数据源/关键词/时间范围筛选。
func (s *AlertService) ListAlerts(p AlertListParams) (*AlertListResult, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 || p.PageSize > 200 {
		p.PageSize = 20
	}
	q := s.db.Model(&model.Alert{})
	if p.Level != "" {
		q = q.Where("level = ?", p.Level)
	}
	if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}
	if p.SourceID > 0 {
		q = q.Where("source_id = ?", p.SourceID)
	}
	if p.ResInstID > 0 {
		q = q.Where("res_inst_id = ?", p.ResInstID)
	}
	if p.Days > 0 {
		q = q.Where("alert_time >= ?", time.Now().AddDate(0, 0, -p.Days))
	}
	if kw := strings.TrimSpace(p.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("rule_name LIKE ? OR instance_name LIKE ? OR instance_id LIKE ? OR metric_name LIKE ?", like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	order := "alert_time desc, id desc"
	if p.Sort == "activity" {
		// 最近动静：取触发时间与恢复时间中较晚者倒序（resolved_at 为空时用触发时间）
		order = "GREATEST(alert_time, COALESCE(resolved_at, alert_time)) desc, id desc"
	}
	var list []model.Alert
	if err := q.Order(order).
		Limit(p.PageSize).Offset((p.Page - 1) * p.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	// 填充数据源名
	var dss []model.AlertSource
	s.db.Select("id", "name").Find(&dss)
	nameByID := map[uint]string{}
	for _, d := range dss {
		nameByID[d.ID] = d.Name
	}
	for i := range list {
		list[i].SourceName = nameByID[list[i].SourceID]
	}
	s.fillAlertResLabels(list)
	return &AlertListResult{List: list, Total: total, Page: p.Page, PageSize: p.PageSize}, nil
}

// fillAlertResLabels 对已关联 CMDB 实例的告警，用实例主标识补 InstanceName（云端为空时）。
func (s *AlertService) fillAlertResLabels(list []model.Alert) {
	ids := map[uint]bool{}
	for _, a := range list {
		if a.ResInstID > 0 {
			ids[a.ResInstID] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	var insts []model.Instance
	s.db.Where("id IN ?", uintKeysT(ids)).Find(&insts)
	instByID := map[uint]model.Instance{}
	modelIDs := map[uint]bool{}
	for _, it := range insts {
		instByID[it.ID] = it
		modelIDs[it.ModelID] = true
	}
	// 每个模型的首个启用字段（主标识）
	var attrs []model.Attribute
	s.db.Where("model_id IN ? AND enabled = ?", uintKeysT(modelIDs), true).Order("`index` asc, id asc").Find(&attrs)
	firstProp := map[uint]string{}
	for _, at := range attrs {
		if _, ok := firstProp[at.ModelID]; !ok {
			firstProp[at.ModelID] = at.PropertyID
		}
	}
	for i := range list {
		if list[i].InstanceName != "" || list[i].ResInstID == 0 {
			continue
		}
		it, ok := instByID[list[i].ResInstID]
		if !ok {
			continue
		}
		var data map[string]interface{}
		_ = json.Unmarshal(it.Data, &data)
		if p := firstProp[it.ModelID]; p != "" {
			if v := optStr(data[p]); v != "" {
				list[i].InstanceName = v
			}
		}
	}
}

// CleanupAlerts 清理告警：days<=0 清空（受 sourceID 限定），否则删除 N 天前的；
// sourceID>0 时仅清理该数据源的告警。
func (s *AlertService) CleanupAlerts(days int, sourceID uint) (int64, error) {
	q := s.db.Session(&gorm.Session{AllowGlobalUpdate: true})
	if days > 0 {
		q = q.Where("alert_time < ?", time.Now().AddDate(0, 0, -days))
	}
	if sourceID > 0 {
		q = q.Where("source_id = ?", sourceID)
	}
	res := q.Delete(&model.Alert{})
	return res.RowsAffected, res.Error
}

// syncTencentAlerts 调用腾讯云监控 DescribeAlarmHistories(2018-07-24) 拉取告警历史。
func (s *AlertService) syncTencentAlerts(ds *model.AlertSource) (int, error) {
	region := firstTencentAlertRegion(ds.Region)
	now := time.Now()
	start := alertWindowStart(ds, now)

	total := 0
	collected := []*model.Alert{}
	for page := 1; page <= 50; page++ {
		payload := map[string]interface{}{
			"Module":     "monitor",
			"PageNumber": page,
			"PageSize":   100,
			"Order":      "DESC",
			"StartTime":  start.Unix(),
			"EndTime":    now.Unix(),
		}
		raw, err := tcAPI("monitor", "DescribeAlarmHistories", "2018-07-24", region, ds.Username, ds.Password, payload)
		if err != nil {
			return total, err
		}
		var wrap map[string]interface{}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return total, err
		}
		resp, _ := wrap["Response"].(map[string]interface{})
		if resp == nil {
			return total, newValidationErr("腾讯云返回异常")
		}
		if e, ok := resp["Error"].(map[string]interface{}); ok {
			return total, newValidationErr("腾讯云接口错误：%s", optStr(e["Message"]))
		}
		histories, _ := resp["Histories"].([]interface{})
		if page == 1 && len(histories) > 0 { // 临时调试：打印首条，排查实例/维度字段
			b, _ := json.Marshal(histories[0])
			fmt.Printf("[alert-debug-tc] first raw item: %s\n", b)
		}
		if len(histories) == 0 {
			break
		}
		for _, item := range histories {
			h, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			alarmID := optStr(h["AlarmId"])
			if alarmID == "" {
				continue
			}
			st := optStr(h["AlarmStatus"])
			status := "resolved"
			if st == "ALARM" || st == "NO_DATA" {
				status = "firing"
			}
			at := now
			if sec, e := strconv.ParseInt(optStr(h["FirstOccurTime"]), 10, 64); e == nil && sec > 0 {
				at = time.Unix(sec, 0)
			}
			var resolvedAt *time.Time
			if status == "resolved" {
				rt := at
				if sec, e := strconv.ParseInt(optStr(h["RecoverTime"]), 10, 64); e == nil && sec > 0 {
					rt = time.Unix(sec, 0)
				}
				resolvedAt = &rt
			}
			metric := ""
			if mi, ok := h["MetricsInfo"].([]interface{}); ok && len(mi) > 0 {
				if m0, ok := mi[0].(map[string]interface{}); ok {
					metric = optStr(m0["MetricName"])
				}
			}
			al := model.Alert{
				SourceID:     ds.ID,
				LogID:        alarmID,
				Level:        "warning", // 腾讯云告警历史无等级字段，统一记为警告
				Status:       status,
				RuleName:     optStr(h["PolicyName"]),
				Product:      optStr(h["Namespace"]),
				MetricName:   metric,
				InstanceID:   optStr(h["OriginId"]), // 原始对象 id（关联用，待真实数据确认）
				InstanceName: optStr(h["AlarmObject"]),
				Dimensions:   parseAlertDimensions(h["Dimensions"]),
				Expression:   optStr(h["Content"]),
				GroupName:    optStr(h["ProjectName"]),
				AlertTime:    at,
				ResolvedAt:   resolvedAt,
			}
			collected = append(collected, &al)
			total++
		}
		if len(histories) < 100 {
			break
		}
	}
	// 先触发后恢复逐条写入：恢复日志就地更新对应触发行的恢复时间，不新增数据
	s.upsertAlertsDedup(collected)
	return total, nil
}

// firstTencentAlertRegion 从数据源地域取首个，默认 ap-guangzhou。
func firstTencentAlertRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "ap-guangzhou"
	}
	if i := strings.IndexByte(region, ','); i >= 0 {
		return strings.TrimSpace(region[:i])
	}
	return region
}

// aliyunExtended 从阿里云告警日志的 ExtendedInfo([]{Name,Value}) 里按 Name 取值。
func aliyunExtended(a map[string]interface{}, name string) string {
	arr, _ := a["ExtendedInfo"].([]interface{})
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok && optStr(m["Name"]) == name {
			return optStr(m["Value"])
		}
	}
	return ""
}

// aliyunAlertID 取阿里云 LogId 中 '[' 之前的 alertId 作为一次告警的稳定去重键。
func aliyunAlertID(logID string) string {
	if i := strings.IndexByte(logID, '['); i > 0 {
		return logID[:i]
	}
	return logID
}

// firstAlertRegion 从数据源地域（可能逗号分隔多地域）取首个，默认 cn-hangzhou。
func firstAlertRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "cn-hangzhou"
	}
	if i := strings.IndexByte(region, ','); i >= 0 {
		return strings.TrimSpace(region[:i])
	}
	return region
}
