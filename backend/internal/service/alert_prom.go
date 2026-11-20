package service

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// promAlertLevel severity 标签 → 统一级别。
func promAlertLevel(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "error", "page":
		return "critical"
	case "warning", "warn":
		return "warning"
	default:
		return "info"
	}
}

// alertFingerprint 由排序后的标签生成稳定去重键（同一告警实例跨轮询一致）。
func alertFingerprint(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte(';')
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// cleanPromDim 组装维度文本：摘要 + namespace/pod。
func cleanPromDim(summary, ns, pod string) string {
	parts := []string{}
	if ns != "" {
		parts = append(parts, "namespace="+ns)
	}
	if pod != "" {
		parts = append(parts, "pod="+pod)
	}
	head := strings.Join(parts, " · ")
	summary = strings.TrimSpace(summary)
	out := summary
	if summary != "" && head != "" {
		out = summary + " · " + head
	} else if summary == "" {
		out = head
	}
	if len(out) > 500 {
		out = out[:500]
	}
	return out
}

// promAlertsGet 拉取 Prometheus /api/v1/alerts 的活跃告警。
func promAlertsGet(ds *model.AlertSource) ([]struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    time.Time         `json:"activeAt"`
	Value       string            `json:"value"`
}, error) {
	base := strings.TrimRight(strings.TrimSpace(ds.URL), "/")
	if base == "" {
		return nil, newValidationErr("Prometheus 地址未配置")
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/alerts", nil)
	if err != nil {
		return nil, err
	}
	if ds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+ds.Token)
	} else if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}
	resp, err := noProxyClient(12 * time.Second).Do(req)
	if err != nil {
		return nil, newValidationErr("请求 Prometheus 失败：%s", err.Error())
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Alerts []struct {
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
				State       string            `json:"state"`
				ActiveAt    time.Time         `json:"activeAt"`
				Value       string            `json:"value"`
			} `json:"alerts"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, newValidationErr("解析 Prometheus 告警失败：%s", err.Error())
	}
	if out.Status != "success" {
		return nil, newValidationErr("Prometheus 返回异常：%s", out.Status)
	}
	return out.Data.Alerts, nil
}

// syncPrometheusAlerts 拉取自建 Prometheus /api/v1/alerts 的 firing 告警并入库，关联资源
// （K8s 应用级：namespace+pod→k8s_app；节点级：instance IP→主机）。本次未出现的旧 firing 置为已恢复。
func (s *AlertService) syncPrometheusAlerts(ds *model.AlertSource) (int, error) {
	alerts, err := promAlertsGet(ds)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	collected := []*model.Alert{}
	current := map[string]bool{}
	for _, a := range alerts {
		if a.State != "firing" { // pending 不计
			continue
		}
		lb := a.Labels
		name := lb["alertname"]
		if name == "" {
			continue
		}
		fp := alertFingerprint(lb)
		current[fp] = true
		ns := lb["namespace"]
		pod := lb["pod"]
		workload := firstNonEmpty(lb["deployment"], lb["workload"], lb["statefulset"], lb["daemonset"])
		at := a.ActiveAt
		if at.IsZero() {
			at = now
		}
		dim := firstNonEmpty(a.Annotations["summary"], a.Annotations["description"], a.Annotations["message"])
		collected = append(collected, &model.Alert{
			SourceID:     ds.ID,
			LogID:        fp,
			Level:        promAlertLevel(lb["severity"]),
			Status:       "firing",
			RuleName:     name,
			Product:      "K8s",
			InstanceID:   firstNonEmpty(pod, workload, lb["instance"]),
			InstanceName: firstNonEmpty(workload, pod, lb["instance"]),
			Dimensions:   cleanPromDim(dim, ns, pod),
			TriggerValue: strings.TrimSpace(a.Value),
			AlertTime:    at,
			Namespace:    ns, // 以下三项仅用于关联 k8s_app（不入库）
			Pod:          pod,
			Workload:     workload,
		})
	}
	s.upsertAlertsDedup(collected)
	resolved := s.resolvePromMissing(ds.ID, current, now)
	return len(collected) + resolved, nil
}

// resolvePromMissing 把本源仍记 firing 但不在当前活跃集合里的告警置为已恢复。
func (s *AlertService) resolvePromMissing(sourceID uint, current map[string]bool, now time.Time) int {
	var firing []model.Alert
	s.db.Where("source_id = ? AND status = ?", sourceID, "firing").Find(&firing)
	cnt := 0
	for _, a := range firing {
		if current[a.LogID] {
			continue
		}
		s.db.Model(&model.Alert{}).Where("id = ?", a.ID).
			Updates(map[string]interface{}{"status": "resolved", "resolved_at": now})
		cnt++
	}
	return cnt
}
