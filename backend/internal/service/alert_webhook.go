package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// genWebhookToken 生成 webhook 校验用随机 token。
func genWebhookToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IngestAlertWebhook 接收任意平台的告警回调（夜莺 / Prometheus Alertmanager / Grafana /
// 自定义脚本等），校验 token 后自动识别 payload 格式并写入告警。
func (s *AlertService) IngestAlertWebhook(sourceID uint, token string, body []byte) (int, error) {
	var ds model.AlertSource
	if s.db.First(&ds, sourceID).Error != nil {
		return 0, ErrNotFound
	}
	if ds.Type != model.AlertSourceWebhook {
		return 0, newValidationErr("非告警回调类型")
	}
	if ds.Token == "" || ds.Token != token {
		return 0, newValidationErr("token 校验失败")
	}
	if !ds.Enabled {
		return 0, newValidationErr("告警源已禁用")
	}

	fmt.Printf("[alert-webhook] source=%d raw: %s\n", sourceID, string(body)) // 临时调试：校准 ARMS 等平台 payload 字段

	alerts := parseWebhookAlerts(ds.ID, body)
	s.upsertAlertsDedup(alerts)
	s.writeLastAlert(ds.ID, fmt.Sprintf("webhook 收到 %d 条告警", len(alerts)))
	return len(alerts), nil
}

// upsertAlertsDedup 批量写入告警，按事件实际发生时间顺序处理，确保恢复只匹配到当时正在 firing 的触发行。
// 排序键：触发事件用触发时间，恢复事件用恢复时间。这样「触发→恢复→再触发」能正确记成两条：
// 先建首次触发、再用恢复时间就地更新它，第二次触发时首条已 resolved、无 firing 可匹配，另起新行。
func (s *AlertService) upsertAlertsDedup(alerts []*model.Alert) {
	eventTime := func(a *model.Alert) time.Time {
		if a.Status == "resolved" && a.ResolvedAt != nil {
			return *a.ResolvedAt
		}
		return a.AlertTime
	}
	sort.SliceStable(alerts, func(i, j int) bool {
		return eventTime(alerts[i]).Before(eventTime(alerts[j]))
	})
	for _, al := range alerts {
		s.resolveAlertResource(al)
		s.upsertAlertDedup(al)
	}
}

// upsertAlertDedup 单条告警写入（去重 + 恢复就地更新）：
//   - 同 (source_id, log_id) 已存在 → 原地更新该行；
//   - 恢复事件(status=resolved，新 log_id，如夜莺/云厂商的恢复日志)：
//       1) 把匹配的触发行(同数据源+实例+规则且仍 firing)就地翻成 resolved，不新增；
//       2) 若已存在"同一时刻恢复过"的行（云厂商周期重复拉取同一条 OK 日志），直接跳过、不重复新建；
//   - 都没匹配到才新建（孤立恢复事件不丢）。
func (s *AlertService) upsertAlertDedup(al *model.Alert) {
	var existing model.Alert
	if s.db.Where("source_id = ? AND log_id = ?", al.SourceID, al.LogID).First(&existing).Error == nil {
		al.ID = existing.ID
		al.CreatedAt = existing.CreatedAt
		// 恢复事件不要把级别覆盖成 ok，保留原有告警级别（critical/warning/info）
		if al.Status == "resolved" && existing.Level != "" && existing.Level != "ok" {
			al.Level = existing.Level
		}
		// 触发条件/触发值为空时不覆盖已有值（如 ARMS 告警恢复后查不到条件，保留触发时存的）
		if al.Expression == "" {
			al.Expression = existing.Expression
		}
		if al.TriggerValue == "" {
			al.TriggerValue = existing.TriggerValue
		}
		s.db.Model(&model.Alert{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"level": al.Level, "status": al.Status, "rule_name": al.RuleName,
			"product": al.Product, "metric_name": al.MetricName,
			"instance_id": al.InstanceID, "instance_name": al.InstanceName,
			"dimensions": al.Dimensions, "res_inst_id": al.ResInstID, "res_model": al.ResModel,
			"expression": al.Expression, "trigger_value": al.TriggerValue, "group_name": al.GroupName,
			"alert_time": al.AlertTime, "resolved_at": al.ResolvedAt,
		})
		return
	}
	// firing 事件去重：同数据源+规则+实例已有仍 firing 的行时，就地刷新（含 log_id、触发时间），
	// 不新建——夜莺等平台对持续未恢复的告警会用新事件 id 重复通知。
	if al.Status == "firing" && al.RuleName != "" {
		q := s.db.Model(&model.Alert{}).
			Where("source_id = ? AND rule_name = ? AND status = ?", al.SourceID, al.RuleName, "firing")
		if al.InstanceID != "" {
			q = q.Where("instance_id = ?", al.InstanceID)
		}
		var ex model.Alert
		if q.Order("id asc").First(&ex).Error == nil {
			if al.Expression == "" {
				al.Expression = ex.Expression
			}
			if al.TriggerValue == "" {
				al.TriggerValue = ex.TriggerValue
			}
			s.db.Model(&model.Alert{}).Where("id = ?", ex.ID).Updates(map[string]interface{}{
				"log_id": al.LogID, "level": al.Level, "product": al.Product, "metric_name": al.MetricName,
				"instance_id": al.InstanceID, "instance_name": al.InstanceName, "dimensions": al.Dimensions,
				"res_inst_id": al.ResInstID, "res_model": al.ResModel, "expression": al.Expression,
				"trigger_value": al.TriggerValue, "group_name": al.GroupName, "alert_time": al.AlertTime,
			})
			return
		}
	}
	if al.Status == "resolved" && al.RuleName != "" {
		rt := al.ResolvedAt
		if rt == nil {
			rt = &al.AlertTime
		}
		// 同数据源+规则(+实例) 的条件，每次取全新查询，避免链式条件污染
		where := func() *gorm.DB {
			q := s.db.Model(&model.Alert{}).Where("source_id = ? AND rule_name = ?", al.SourceID, al.RuleName)
			if al.InstanceID != "" {
				q = q.Where("instance_id = ?", al.InstanceID)
			}
			return q
		}
		// 1) 仍 firing 的触发行 → 翻成 resolved（首次恢复）
		if res := where().Where("status = ?", "firing").
			Updates(map[string]interface{}{"status": "resolved", "resolved_at": rt}); res.Error == nil && res.RowsAffected > 0 {
			return
		}
		// 2) 已按同一时刻恢复过（周期重复拉取同一条 OK 日志）→ 跳过，不重复新建
		var cnt int64
		where().Where("status = ? AND resolved_at = ?", "resolved", rt).Count(&cnt)
		if cnt > 0 {
			return
		}
	}
	s.db.Create(al)
}

// parseWebhookAlerts 自动识别回调 payload 的格式并解析为告警列表：
//   - Prometheus Alertmanager：{alerts:[{labels,annotations,status,...}]}
//   - Grafana(legacy)：{ruleName,state,evalMatches:[...]}
//   - 夜莺 / 通用事件：单对象 / 数组 / {event} / {events} 包裹，字段名做多别名兼容
func parseWebhookAlerts(sourceID uint, body []byte) []*model.Alert {
	var v interface{}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		if arr, ok := m["alerts"].([]interface{}); ok && looksAlertmanager(arr) {
			out := []*model.Alert{}
			for _, it := range arr {
				if em, ok := it.(map[string]interface{}); ok {
					if al := buildAlertmanagerAlert(sourceID, em, m); al != nil {
						out = append(out, al)
					}
				}
			}
			return out
		}
		if _, ok := m["evalMatches"]; ok {
			if al := buildGrafanaAlert(sourceID, m); al != nil {
				return []*model.Alert{al}
			}
			return nil
		}
	}
	out := []*model.Alert{}
	for _, e := range collectN9eEvents(v) {
		if al := buildEventAlert(sourceID, e); al != nil {
			out = append(out, al)
		}
	}
	return out
}

// collectN9eEvents 从 body 里提取事件列表（兼容 单对象 / 数组 / {event}/{events} 包裹）。
func collectN9eEvents(v interface{}) []map[string]interface{} {
	switch t := v.(type) {
	case []interface{}:
		out := []map[string]interface{}{}
		for _, it := range t {
			out = append(out, collectN9eEvents(it)...)
		}
		return out
	case map[string]interface{}:
		if ev, ok := t["event"].(map[string]interface{}); ok {
			return []map[string]interface{}{ev}
		}
		if evs, ok := t["events"].([]interface{}); ok {
			return collectN9eEvents(evs)
		}
		return []map[string]interface{}{t}
	}
	return nil
}

// buildEventAlert 把一个事件 map 转成告警记录，字段名做多别名兼容（夜莺 + 通用）。
func buildEventAlert(sourceID uint, e map[string]interface{}) *model.Alert {
	ruleName := firstStr(e, "rule_name", "rulename", "alertname", "alert_name", "name", "title")
	ident := firstStr(e, "target_ident", "instance", "host", "hostname", "ident", "ip", "resource", "object")
	if ident == "" {
		// 夜莺 prom 类规则 target_ident 常为空，真实主机在 tags/tags_map 里
		ident = identFromTags(e)
	}
	ident = stripPort(ident)
	// k8s 告警：用 pod 名作实例标识，而非容器 IP
	if pod := podFromEvent(e); pod != "" {
		ident = pod
	}
	id := firstStr(e, "id", "event_id", "eventid", "fingerprint", "alert_id", "alertid")
	if id == "" || id == "0" {
		// 兜底唯一键：规则 + 对象 + 首次触发时间
		id = ruleName + "|" + ident + "|" + firstStr(e, "trigger_time", "first_trigger_time", "startsAt", "timestamp", "time")
	}
	if strings.Trim(id, "| ") == "" {
		return nil
	}
	recovered := false
	if b, ok := e["is_recovered"].(bool); ok {
		recovered = b
	} else {
		switch strings.ToLower(firstStr(e, "status", "state", "event_status")) {
		case "resolved", "ok", "recovered", "normal", "cleared":
			recovered = true
		}
	}
	// 恢复事件与阿里云 OK 行对齐：level=ok / status=resolved
	status := "firing"
	level := normalizeLevel(firstStr(e, "severity", "level", "priority", "prio"))
	// 触发时间始终取首次触发时刻；恢复时间单独记录（夜莺恢复的真实时刻在 last_eval_time）
	// 触发时间取最新一次触发(trigger_time)，而非最初(first_trigger_time)，使重复触发能刷新
	at := parseFlexTime(firstVal(e, "trigger_time", "startsAt", "timestamp", "time", "alert_time", "first_trigger_time"))
	if at.IsZero() {
		at = time.Now()
	}
	var resolvedAt *time.Time
	if recovered {
		status = "resolved"
		// 级别保留 severity（不强制 ok），恢复只体现在 status/resolved_at
		rt := parseFlexTime(firstVal(e, "recover_time", "last_eval_time", "end_time", "endsAt", "resolve_time"))
		if rt.IsZero() {
			rt = time.Now()
		}
		resolvedAt = &rt
	}
	dims := n9eTags(e["tags"])
	if dims == "" {
		if lm, ok := e["labels"].(map[string]interface{}); ok {
			dims = labelDims(lm, "alertname", "severity", "__name__")
		}
	}
	product := firstStr(e, "product", "cluster")
	if product == "" {
		if _, ok := e["prom_ql"]; ok {
			product = "Nightingale"
		} else if _, ok := e["target_ident"]; ok {
			product = "Nightingale"
		} else {
			product = "Webhook"
		}
	}
	return &model.Alert{
		SourceID:     sourceID,
		LogID:        id,
		Level:        level,
		Status:       status,
		RuleName:     ruleName,
		Product:      product,
		MetricName:   firstStr(e, "metric", "__name__", "metric_name"),
		InstanceID:   ident,
		InstanceName: ident,
		Dimensions:   dims,
		Expression:   firstStr(e, "prom_ql", "promql", "expression", "query", "description"),
		TriggerValue: n9eTriggerValue(e),
		GroupName:    firstStr(e, "group_name", "busi_group", "groupname"),
		AlertTime:    at,
		ResolvedAt:   resolvedAt,
		Cluster:      clusterFromEvent(e),
	}
}

// clusterFromEvent 从 n9e/通用事件(tags_map/labels/tags 数组/直接字段) 取 k8s 集群标识。
func clusterFromEvent(e map[string]interface{}) string {
	keys := []string{"cluster", "cluster_name", "origin_prometheus", "k8s_cluster"}
	if tm, ok := e["tags_map"].(map[string]interface{}); ok {
		if v := clusterLabel(tm); v != "" {
			return v
		}
	}
	if lm, ok := e["labels"].(map[string]interface{}); ok {
		if v := clusterLabel(lm); v != "" {
			return v
		}
	}
	if arr, ok := e["tags"].([]interface{}); ok {
		for _, k := range keys {
			prefix := k + "="
			for _, it := range arr {
				s := strings.TrimSpace(optStr(it))
				if strings.HasPrefix(s, prefix) {
					return strings.TrimSpace(strings.TrimPrefix(s, prefix))
				}
			}
		}
	}
	return firstStr(e, keys...)
}

// n9eTriggerValue 从夜莺事件取「触发时的实际值」，优先已格式化的 text，再退回 trigger_value。
func n9eTriggerValue(e map[string]interface{}) string {
	if tj, ok := e["trigger_values_json"].(map[string]interface{}); ok {
		if vu, ok := tj["values_with_unit"].(map[string]interface{}); ok {
			keys := make([]string, 0, len(vu))
			for k := range vu {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := []string{}
			for _, k := range keys {
				m, ok := vu[k].(map[string]interface{})
				if !ok {
					continue
				}
				t := strings.TrimSpace(optStr(m["text"]))
				if t == "" {
					t = trimFloat(optStr(m["value"]))
				}
				if u := strings.TrimSpace(optStr(m["unit"])); u != "" && t != "" {
					t += u
				}
				if t != "" {
					parts = append(parts, t)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ", ")
			}
		}
	}
	return trimFloat(firstStr(e, "trigger_value", "trigger_values"))
}

// trimFloat 把数值串格式化为最多两位小数并去掉多余的 0（如 90.01515→90.02、1.00→1）。
func trimFloat(s string) string {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	r := strconv.FormatFloat(f, 'f', 2, 64)
	if strings.Contains(r, ".") {
		r = strings.TrimRight(strings.TrimRight(r, "0"), ".")
	}
	return r
}

// looksAlertmanager 判断 alerts 数组是否为 Prometheus Alertmanager 的结构。
func looksAlertmanager(arr []interface{}) bool {
	if len(arr) == 0 {
		return false
	}
	m, ok := arr[0].(map[string]interface{})
	if !ok {
		return false
	}
	if _, ok := m["labels"]; ok {
		return true
	}
	if _, ok := m["fingerprint"]; ok {
		return true
	}
	return false
}

// buildAlertmanagerAlert 解析 Prometheus Alertmanager 的单条告警。
func buildAlertmanagerAlert(sourceID uint, a, root map[string]interface{}) *model.Alert {
	labels, _ := a["labels"].(map[string]interface{})
	ann, _ := a["annotations"].(map[string]interface{})
	ruleName := mstr(labels, "alertname")
	if ruleName == "" {
		ruleName = mstr(ann, "summary")
	}
	ident := mstr(labels, "instance")
	if ident == "" {
		ident = mstr(labels, "host")
	}
	ident = stripPort(ident)
	// k8s 告警：用 pod 名作实例标识，而非容器 IP
	if pod := podFromLabels(labels); pod != "" {
		ident = pod
	}
	status := strings.ToLower(optStr(a["status"]))
	if status == "" {
		status = strings.ToLower(optStr(root["status"]))
	}
	if status != "resolved" {
		status = "firing"
	}
	id := optStr(a["fingerprint"])
	if id == "" {
		id = ruleName + "|" + ident + "|" + optStr(a["startsAt"])
	}
	if strings.Trim(id, "| ") == "" {
		return nil
	}
	at := parseFlexTime(a["startsAt"])
	if at.IsZero() {
		at = time.Now()
	}
	level := normalizeLevel(mstr(labels, "severity"))
	var resolvedAt *time.Time
	if status == "resolved" {
		// 保留 severity 级别，恢复只体现在 status/resolved_at
		rt := parseFlexTime(a["endsAt"])
		if rt.IsZero() {
			rt = time.Now()
		}
		resolvedAt = &rt
	}
	expr := mstr(ann, "description")
	if expr == "" {
		expr = optStr(a["generatorURL"])
	}
	return &model.Alert{
		SourceID:     sourceID,
		LogID:        id,
		Level:        level,
		Status:       status,
		RuleName:     ruleName,
		Product:      "Alertmanager",
		MetricName:   mstr(labels, "__name__"),
		InstanceID:   ident,
		InstanceName: ident,
		Dimensions:   labelDims(labels, "alertname", "severity", "__name__"),
		Expression:   expr,
		AlertTime:    at,
		ResolvedAt:   resolvedAt,
		Cluster:      clusterLabel(labels),
	}
}

// clusterLabel 从 label map 取 k8s 集群标识（Prometheus external_labels 常用 cluster/origin_prometheus）。
func clusterLabel(m map[string]interface{}) string {
	for _, k := range []string{"cluster", "cluster_name", "origin_prometheus", "k8s_cluster"} {
		if v := mstr(m, k); v != "" {
			return v
		}
	}
	return ""
}

// buildGrafanaAlert 解析 Grafana(legacy) webhook 告警。
func buildGrafanaAlert(sourceID uint, m map[string]interface{}) *model.Alert {
	ruleName := optStr(m["ruleName"])
	if ruleName == "" {
		ruleName = optStr(m["title"])
	}
	state := strings.ToLower(optStr(m["state"]))
	status := "firing"
	level := "warning"
	var resolvedAt *time.Time
	if state == "ok" || state == "no_data" || state == "paused" {
		status = "resolved"
		now := time.Now()
		resolvedAt = &now
	}
	id := optStr(m["ruleId"])
	if id == "" || id == "0" {
		id = ruleName
	}
	if strings.TrimSpace(id) == "" {
		return nil
	}
	dims := []string{}
	if arr, ok := m["evalMatches"].([]interface{}); ok {
		for _, it := range arr {
			if em, ok := it.(map[string]interface{}); ok {
				metric := optStr(em["metric"])
				if metric != "" {
					dims = append(dims, metric+"="+optStr(em["value"]))
				}
			}
		}
	}
	return &model.Alert{
		SourceID:   sourceID,
		LogID:      id,
		Level:      level,
		Status:     status,
		RuleName:   ruleName,
		Product:    "Grafana",
		Dimensions: strings.Join(dims, " · "),
		Expression: optStr(m["message"]),
		AlertTime:  time.Now(),
		ResolvedAt: resolvedAt,
	}
}

// n9eTags 事件 tags（["k=v",...] 或 map）→ 可读维度串，过滤内部标签。
func n9eTags(v interface{}) string {
	if arr, ok := v.([]interface{}); ok {
		parts := []string{}
		for _, it := range arr {
			s := strings.TrimSpace(optStr(it))
			if s == "" || strings.HasPrefix(s, "__") {
				continue
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, " · ")
	}
	return parseAlertDimensions(v)
}

// normalizeLevel 把各平台的级别/严重度统一成 critical / warning / info。
func normalizeLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "critical", "crit", "disaster", "emergency", "fatal", "error", "p0", "p1":
		return "critical"
	case "2", "warning", "warn", "high", "average", "p2":
		return "warning"
	case "3", "info", "information", "informational", "notice", "low", "p3", "p4":
		return "info"
	default:
		return "warning"
	}
}

// labelDims 把 label map 拼成稳定排序的 "k=v · k=v" 维度串，过滤指定键与 "__" 内部键。
func labelDims(m map[string]interface{}, exclude ...string) string {
	if m == nil {
		return ""
	}
	skip := map[string]bool{}
	for _, e := range exclude {
		skip[e] = true
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if skip[k] || strings.HasPrefix(k, "__") {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		v := strings.TrimSpace(optStr(m[k]))
		if v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " · ")
}

// parseFlexTime 尽量把各种时间表示解析为 time.Time：unix 秒/毫秒、RFC3339、常见日期串。
func parseFlexTime(v interface{}) time.Time {
	switch t := v.(type) {
	case float64:
		return unixFlex(int64(t))
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return unixFlex(n)
		}
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return unixFlex(n)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if tm, err := time.Parse(layout, s); err == nil {
				return tm
			}
		}
	}
	return time.Time{}
}

func unixFlex(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	if n > 1e12 { // 毫秒级
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}

// firstStr 返回 m 中 keys 里第一个非空字符串值。
func firstStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := strings.TrimSpace(optStr(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// firstVal 返回 m 中 keys 里第一个非空值（字符串空白视为空）。
func firstVal(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, isStr := v.(string); isStr && strings.TrimSpace(s) == "" {
				continue
			}
			return v
		}
	}
	return nil
}

// identFromTags 当 target_ident 为空时，从事件 tags_map / tags(["k=v"]) 里兜底提取主机标识。
// podFromLabels 从 label map 取 pod 名（k8s 告警优先用 pod 名作实例标识，而非容器 IP）。
func podFromLabels(m map[string]interface{}) string {
	for _, k := range []string{"pod", "pod_name", "podname"} {
		if v := mstr(m, k); v != "" {
			return v
		}
	}
	return ""
}

// podFromEvent 从 n9e/通用事件(tags_map/labels/tags 数组/直接字段) 取 pod 名。
func podFromEvent(e map[string]interface{}) string {
	keys := []string{"pod", "pod_name", "podname"}
	if tm, ok := e["tags_map"].(map[string]interface{}); ok {
		if v := podFromLabels(tm); v != "" {
			return v
		}
	}
	if lm, ok := e["labels"].(map[string]interface{}); ok {
		if v := podFromLabels(lm); v != "" {
			return v
		}
	}
	if arr, ok := e["tags"].([]interface{}); ok {
		for _, k := range keys {
			prefix := k + "="
			for _, it := range arr {
				s := strings.TrimSpace(optStr(it))
				if strings.HasPrefix(s, prefix) {
					return strings.TrimSpace(strings.TrimPrefix(s, prefix))
				}
			}
		}
	}
	return firstStr(e, "pod", "pod_name")
}

func identFromTags(e map[string]interface{}) string {
	keys := []string{"instance", "ident", "host", "hostname", "ip"}
	if tm, ok := e["tags_map"].(map[string]interface{}); ok {
		for _, k := range keys {
			if v := mstr(tm, k); v != "" {
				return v
			}
		}
	}
	if arr, ok := e["tags"].([]interface{}); ok {
		for _, k := range keys {
			prefix := k + "="
			for _, it := range arr {
				s := strings.TrimSpace(optStr(it))
				if strings.HasPrefix(s, prefix) {
					return strings.TrimSpace(strings.TrimPrefix(s, prefix))
				}
			}
		}
	}
	return ""
}

// stripPort 去掉 "host:port" 尾部端口（如 10.0.0.1:9100 → 10.0.0.1），便于与 CMDB 主机关联。
func stripPort(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, ":"); i > 0 {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			return s[:i]
		}
	}
	return s
}

// mstr 从 label/annotation map 取字符串值（nil 安全）。
func mstr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(optStr(m[key]))
}
