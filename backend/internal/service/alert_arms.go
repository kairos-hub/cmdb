package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// armsCondCache 累积「AlertName → 触发条件」。ListActivatedAlerts 只返回活跃告警，
// 告警恢复后其条件就查不到了；用进程级缓存记住学到过的条件，供已恢复告警回填。
var armsCondCache sync.Map

// armsAlertLevel ARMS 告警等级(P1..P6) → 统一级别 id。
func armsAlertLevel(sev string) string {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "P1":
		return "critical"
	case "P2", "P3":
		return "warning"
	default: // P4/P5/P6/空
		return "info"
	}
}

// armsNum 解析 ARMS 返回里可能是数字或字符串的字段。
func armsNum(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	}
	return 0
}

// cleanArmsDescribe 去掉 Describe 里的「【通知策略: ...】」前缀与换行，保留正文细节。
func cleanArmsDescribe(d string) string {
	d = strings.TrimSpace(d)
	for strings.HasPrefix(d, "【") {
		if i := strings.IndexRune(d, '】'); i >= 0 {
			d = strings.TrimSpace(d[i+len("】"):])
		} else {
			break
		}
	}
	d = strings.ReplaceAll(d, "\n", " ")
	d = strings.Join(strings.Fields(d), " ")
	if len(d) > 500 {
		d = d[:500]
	}
	return d
}

// ackClusterNames 返回 CMDB 中 kubernetes 模型所有实例的 cluster_name 集合（长名优先，避免短名误匹配）。
func (s *AlertService) ackClusterNames() []string {
	k8sID := s.inst.ModelIDByObjID("kubernetes")
	if k8sID == 0 {
		return nil
	}
	var names []string
	s.db.Model(&model.Instance{}).Where("model_id = ?", k8sID).
		Pluck("JSON_UNQUOTE(JSON_EXTRACT(data, '$.cluster_name'))", &names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && n != "null" {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// matchAckCluster 若 AlertName 以 _<已知集群名> 结尾，返回该集群名，否则空串。
func matchAckCluster(alertName string, clusters []string) string {
	for _, c := range clusters {
		if strings.HasSuffix(alertName, "_"+c) {
			return c
		}
	}
	return ""
}

// armsRuleConditions 调 ListAlertEvents，按 AlertName 建「触发条件」表：
// 事件 Labels.metrics_params_metrics_name/opt/value → "指标 opt 阈值"（ListAlerts 不返回条件，故从事件补齐）。
// 用 ListAlertEvents 而非 ListActivatedAlerts：前者含已恢复事件，故已恢复告警也能拿到条件。
func armsRuleConditions(ds *model.AlertSource, region string) map[string]string {
	out := map[string]string{}
	for page := 1; page <= 20; page++ {
		raw, err := aliRPC("https://arms."+region+".aliyuncs.com", ds.Username, ds.Password, "2019-08-08",
			"ListAlertEvents", map[string]string{"RegionId": region, "Page": strconv.Itoa(page), "Size": "100"})
		if err != nil {
			break
		}
		var top struct {
			PageBean struct {
				Events []struct {
					AlertName string `json:"AlertName"`
					Labels    string `json:"Labels"`
				} `json:"Events"`
			} `json:"PageBean"`
		}
		if json.Unmarshal(raw, &top) != nil || len(top.PageBean.Events) == 0 {
			break
		}
		for _, e := range top.PageBean.Events {
			if e.Labels == "" {
				continue
			}
			var lbl map[string]interface{}
			if json.Unmarshal([]byte(e.Labels), &lbl) != nil {
				continue
			}
			mn := optStr(lbl["metrics_params_metrics_name"])
			opt := optStr(lbl["metrics_params_opt"])
			val := optStr(lbl["metrics_params_value"])
			if mn != "" && val != "" {
				armsCondCache.Store(e.AlertName, strings.TrimSpace(mn+" "+opt+" "+val))
			}
		}
		if len(top.PageBean.Events) < 100 {
			break
		}
	}
	// 返回累积缓存的全部条件（含历史学到的，供已恢复告警回填）
	armsCondCache.Range(func(k, v interface{}) bool {
		out[k.(string)] = v.(string)
		return true
	})
	return out
}

// syncArmsAlerts 从阿里云 ARMS 告警管理（ListAlerts）拉取告警，仅保留 ACK 集群相关
// （AlertName 以 _<已知集群名> 结尾），按集群名关联到 kubernetes 集群实例。返回处理条数。
func (s *AlertService) syncArmsAlerts(ds *model.AlertSource) (int, error) {
	clusters := s.ackClusterNames()
	if len(clusters) == 0 {
		return 0, nil // CMDB 无 k8s 集群实例，无需拉取
	}
	now := time.Now()
	start := alertWindowStart(ds, now)
	// ARMS 告警较稀疏，窗口过短会漏掉数小时前触发或仍在告警的事件：至少回看 24 小时。
	if floor := now.Add(-24 * time.Hour); start.After(floor) {
		start = floor
	}
	total := 0
	collected := []*model.Alert{}
	for _, region := range parseRegions(ds.Region) {
		endpoint := "https://arms." + region + ".aliyuncs.com"
		conds := armsRuleConditions(ds, region) // AlertName → 触发条件
		stop := false
		for page := 1; page <= 50 && !stop; page++ {
			raw, err := aliRPC(endpoint, ds.Username, ds.Password, "2019-08-08", "ListAlerts",
				map[string]string{"RegionId": region, "Page": strconv.Itoa(page), "Size": "100"})
			if err != nil {
				return total, err
			}
			var top struct {
				PageBean struct {
					ListAlerts []map[string]interface{} `json:"ListAlerts"`
				} `json:"PageBean"`
				Code    int    `json:"Code"`
				Message string `json:"Message"`
			}
			if err := json.Unmarshal(raw, &top); err != nil {
				return total, err
			}
			if top.Code != 0 && top.Code != 200 && top.Message != "" {
				return total, newValidationErr("ARMS 接口错误：%s", top.Message)
			}
			list := top.PageBean.ListAlerts
			if len(list) == 0 {
				break
			}
			for _, a := range list {
				name := optStr(a["AlertName"])
				cluster := matchAckCluster(name, clusters)
				if cluster == "" {
					continue // 非 ACK 集群告警，丢弃
				}
				at := now
				if t, e := time.ParseInLocation("2006-01-02 15:04:05", optStr(a["CreateTime"]), time.Local); e == nil {
					at = t
				}
				recover := armsNum(a["RecoverTime"]) // -1=未恢复；>=0=恢复耗时(秒)
				firing := armsNum(a["State"]) != 2 && recover < 0
				if at.Before(start) {
					stop = true // 列表按时间倒序，已越过窗口 → 本页处理完即停止翻页
					if !firing {
						continue // 已恢复且早于窗口 → 丢弃；仍在告警的则保留（长期未恢复的活跃告警）
					}
				}
				logID := strconv.FormatInt(int64(armsNum(a["AlertId"])), 10)
				status := "firing"
				var resolvedAt *time.Time
				if !firing {
					status = "resolved"
					rt := at.Add(time.Duration(recover) * time.Second)
					resolvedAt = &rt
				}
				al := model.Alert{
					SourceID:     ds.ID,
					LogID:        logID,
					Level:        armsAlertLevel(optStr(a["Severity"])),
					Status:       status,
					RuleName:     strings.TrimSuffix(name, "_"+cluster), // 去掉集群后缀的规则名
					Product:      "ACK",
					Expression:   conds[name],                              // 触发条件（指标 opt 阈值），来自 ListActivatedAlerts
					Dimensions:   cleanArmsDescribe(optStr(a["Describe"])), // 告警详情
					InstanceID:   cluster,                                  // 以集群名作实例标识（firing 去重按 规则+集群）
					InstanceName: cluster,
					Cluster:      cluster, // 供 resolveAlertResource 关联到 kubernetes 集群实例
					AlertTime:    at,
					ResolvedAt:   resolvedAt,
				}
				collected = append(collected, &al)
				total++
			}
			if len(list) < 100 {
				break
			}
		}
	}
	s.upsertAlertsDedup(collected)
	return total, nil
}
