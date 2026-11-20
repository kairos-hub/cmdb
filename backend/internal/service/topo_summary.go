package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
)

// 业务拓扑总览汇总：供前端大屏展示统计卡片、各模块状态、资源分组分布与异常资源列表。
// 所有数据均来自真实的业务归属 + 资源实例状态，不做任何模拟。

// ModuleStat 单个模块下资源的状态分布。
type ModuleStat struct {
	Normal   int `json:"normal"`   // 正常
	Abnormal int `json:"abnormal"` // 异常
	Offline  int `json:"offline"`  // 离线/未知
	Total    int `json:"total"`
}

// GroupDist 资源按模型分组的数量分布（环形图用）。
type GroupDist struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AbnormalRes 异常/离线资源条目（替代图中无数据源的“告警信息”面板）。
type AbnormalRes struct {
	ID       uint   `json:"id"`
	ModuleID uint   `json:"module_id"` // 所属模块，供前端按业务/集群筛选
	Model    string `json:"model"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	State    string `json:"state"` // abnormal | offline
}

// AlertBrief 业务拓扑里展示的在触发告警（精简）。
type AlertBrief struct {
	ID        uint      `json:"id"`
	ModuleID  uint      `json:"module_id"` // 资源所属模块，供前端按业务筛选
	Level     string    `json:"level"`
	RuleName  string    `json:"rule_name"`
	Label     string    `json:"label"` // 资源名/实例名
	Model     string    `json:"model"`
	ResInstID uint      `json:"res_inst_id"`
	AlertTime time.Time `json:"alert_time"`
}

// BizSummaryResult 业务拓扑总览汇总结果（业务线维度；前端按所选业务再聚合）。
type BizSummaryResult struct {
	BusinessCount int                  `json:"business_count"`
	SetCount      int                  `json:"set_count"`
	ModuleCount   int                  `json:"module_count"`
	ResourceTotal int                  `json:"resource_total"`
	AbnormalTotal int                  `json:"abnormal_total"` // 异常 + 离线
	Modules       map[uint]*ModuleStat `json:"modules"`        // moduleID -> 状态分布
	GroupDist     []GroupDist          `json:"group_dist"`
	Abnormal      []AbnormalRes        `json:"abnormal"`
	AlertTotal    int                  `json:"alert_total"` // 该业务线资源在触发的告警数
	Alerts        []AlertBrief         `json:"alerts"`      // 关联到本业务线资源的在触发告警（含 res_inst_id，前端按业务再筛）
}

// 状态分类：正常 / 离线 / 异常（其余）。
// 正常：running / "1"（主机运行中）/ normal（域名正常）
// 离线：offline（已下线）/ 空值（模型有状态字段但实例未填，视为未知离线）
// 其余（stopped/abnormal/creating/expired/expiring/disabled/"2"…）归异常。
func classifyState(v string) string {
	switch v {
	case "running", "1", "normal":
		return "normal"
	case "offline", "":
		return "offline"
	default:
		return "abnormal"
	}
}

func optStr(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// 枚举 id 可能以数字存储（如主机状态 1/2）
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// parseOptionMap 解析枚举选项 [{id,name}] 为 id->name。
func parseOptionMap(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var opts []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &opts) == nil {
		for _, o := range opts {
			out[o.ID] = o.Name
		}
	}
	return out
}

// BizSummary 汇总某业务的拓扑总览数据。
func (s *TopoService) BizSummary(bizID uint) (*BizSummaryResult, error) {
	var biz model.Instance
	if err := s.db.Where("id = ? AND model_id = ?", bizID, s.bizModelID()).First(&biz).Error; err != nil {
		return nil, ErrNotFound
	}
	res := &BizSummaryResult{
		Modules:   map[uint]*ModuleStat{},
		GroupDist: []GroupDist{},
		Abnormal:  []AbnormalRes{},
	}

	// 业务下的集群/模块
	var nodes []model.TopoNode
	s.db.Where("biz_id = ?", bizID).Order("id asc").Find(&nodes)
	for _, n := range nodes {
		switch n.NodeType {
		case model.TopoBusiness:
			res.BusinessCount++
		case model.TopoSet:
			res.SetCount++
		case model.TopoModule:
			res.ModuleCount++
			res.Modules[n.ID] = &ModuleStat{}
		}
	}

	// 归属关系：hostID -> moduleID
	var hms []model.HostModule
	s.db.Where("biz_id = ?", bizID).Find(&hms)
	hostModule := map[uint]uint{}
	hostIDs := make([]uint, 0, len(hms))
	for _, hm := range hms {
		hostModule[hm.HostID] = hm.ModuleID
		hostIDs = append(hostIDs, hm.HostID)
	}
	if len(hostIDs) == 0 {
		return res, nil
	}

	var insts []model.Instance
	s.db.Where("id IN ?", hostIDs).Find(&insts)
	res.ResourceTotal = len(insts)

	// 涉及的模型元数据
	modelIDs := map[uint]bool{}
	for _, it := range insts {
		modelIDs[it.ModelID] = true
	}
	var models []model.Model
	s.db.Where("id IN ?", uintKeysT(modelIDs)).Find(&models)
	modelName := map[uint]string{}
	modelGroup := map[uint]uint{}
	for _, m := range models {
		modelName[m.ID] = m.Name
		modelGroup[m.ID] = m.GroupID
	}
	var groups []model.ModelGroup
	s.db.Find(&groups)
	groupName := map[uint]string{}
	for _, g := range groups {
		groupName[g.ID] = g.Name
	}

	// 各模型：状态字段 + 用于生成标识的有序启用字段
	type statusMeta struct {
		prop string
		opt  map[string]string
	}
	type labelAttr struct {
		prop, typ string
		opt       map[string]string
	}
	smeta := map[uint]statusMeta{}
	labelAttrs := map[uint][]labelAttr{}
	var attrs []model.Attribute
	s.db.Where("model_id IN ?", uintKeysT(modelIDs)).Order("`index` asc").Find(&attrs)
	for _, a := range attrs {
		opt := parseOptionMap(a.Options)
		if a.PropertyID == "status" || a.PropertyID == "bk_host_status" {
			smeta[a.ModelID] = statusMeta{prop: a.PropertyID, opt: opt}
		}
		if a.Enabled {
			labelAttrs[a.ModelID] = append(labelAttrs[a.ModelID], labelAttr{prop: a.PropertyID, typ: a.Type, opt: opt})
		}
	}

	buildLabel := func(modelID uint, data map[string]interface{}) string {
		parts := []string{}
		for _, a := range labelAttrs[modelID] {
			v, ok := data[a.prop]
			if !ok {
				continue
			}
			sv := optStr(v)
			if sv == "" {
				continue
			}
			if a.typ == model.FieldEnum {
				if name, ok := a.opt[sv]; ok {
					sv = name
				}
			}
			parts = append(parts, sv)
			if len(parts) >= 3 {
				break
			}
		}
		if len(parts) == 0 {
			return ""
		}
		out := parts[0]
		for _, p := range parts[1:] {
			out += " / " + p
		}
		return out
	}

	groupCount := map[string]int{}
	instLabel := map[uint]string{} // 实例 id -> 主标识（用于告警显示名）
	for _, it := range insts {
		var data map[string]interface{}
		_ = json.Unmarshal(it.Data, &data)
		instLabel[it.ID] = buildLabel(it.ModelID, data)

		gname := groupName[modelGroup[it.ModelID]]
		if gname == "" {
			gname = "其他"
		}
		groupCount[gname]++

		// 该模型无状态字段 → 无法判定，一律记正常（不误报异常）
		sm, hasStatus := smeta[it.ModelID]
		state := "normal"
		rawVal := ""
		if hasStatus {
			rawVal = optStr(data[sm.prop])
			state = classifyState(rawVal)
		}

		if ms := res.Modules[hostModule[it.ID]]; ms != nil {
			ms.Total++
			switch state {
			case "normal":
				ms.Normal++
			case "offline":
				ms.Offline++
			default:
				ms.Abnormal++
			}
		}

		if state != "normal" {
			res.AbnormalTotal++
			statusText := sm.opt[rawVal]
			if statusText == "" {
				statusText = rawVal
			}
			if statusText == "" {
				statusText = "未知"
			}
			res.Abnormal = append(res.Abnormal, AbnormalRes{
				ID:       it.ID,
				ModuleID: hostModule[it.ID],
				Model:    modelName[it.ModelID],
				Label:    buildLabel(it.ModelID, data),
				Status:   statusText,
				State:    state, // abnormal | offline
			})
		}
	}

	for name, c := range groupCount {
		res.GroupDist = append(res.GroupDist, GroupDist{Name: name, Count: c})
	}
	sort.Slice(res.GroupDist, func(i, j int) bool {
		if res.GroupDist[i].Count != res.GroupDist[j].Count {
			return res.GroupDist[i].Count > res.GroupDist[j].Count
		}
		return res.GroupDist[i].Name < res.GroupDist[j].Name
	})
	// 异常排在离线前
	sort.SliceStable(res.Abnormal, func(i, j int) bool {
		return res.Abnormal[i].State < res.Abnormal[j].State
	})

	// 本业务线资源的在触发告警（按 res_inst_id 关联；前端按 module_id 再筛到所选业务）
	instIDs := make([]uint, 0, len(insts))
	for _, it := range insts {
		instIDs = append(instIDs, it.ID)
	}
	if len(instIDs) > 0 {
		var cnt int64
		s.db.Model(&model.Alert{}).Where("res_inst_id IN ? AND status = ?", instIDs, "firing").Count(&cnt)
		res.AlertTotal = int(cnt)
		var alerts []model.Alert
		s.db.Where("res_inst_id IN ? AND status = ?", instIDs, "firing").
			Order("alert_time desc").Limit(100).Find(&alerts)
		for _, a := range alerts {
			label := instLabel[a.ResInstID] // 优先用关联到的 CMDB 实例主标识
			if label == "" {
				label = a.InstanceName
			}
			if label == "" {
				label = a.InstanceID
			}
			res.Alerts = append(res.Alerts, AlertBrief{
				ID: a.ID, ModuleID: hostModule[a.ResInstID], Level: a.Level, RuleName: a.RuleName,
				Label: label, Model: a.ResModel, ResInstID: a.ResInstID, AlertTime: a.AlertTime,
			})
		}
	}
	return res, nil
}
