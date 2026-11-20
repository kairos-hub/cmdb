package service

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// hostStatusDist 统计主机按状态(bk_host_status)的分布，状态 id 映射为选项名称。
func (s *StatsService) hostStatusDist(hostModelID uint) []NameCount {
	// 状态选项 id -> 名称
	nameOf := map[string]string{}
	var attr model.Attribute
	if s.db.Where("model_id = ? AND property_id = ?", hostModelID, "bk_host_status").First(&attr).Error == nil {
		var opts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(attr.Options, &opts)
		for _, o := range opts {
			nameOf[o.ID] = o.Name
		}
	}
	type row struct {
		Sid   string
		Count int64
	}
	var rows []row
	s.db.Model(&model.Instance{}).
		Select("JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_host_status')) as sid, count(*) as count").
		Where("model_id = ?", hostModelID).
		Group("sid").Scan(&rows)
	out := []NameCount{}
	for _, r := range rows {
		name := nameOf[r.Sid]
		if name == "" {
			name = "未知"
		}
		out = append(out, NameCount{Name: name, Count: r.Count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// StatsService 概览统计。
type StatsService struct {
	db *gorm.DB
}

func NewStatsService(db *gorm.DB) *StatsService {
	return &StatsService{db: db}
}

// Overview 概览统计数据。
type Overview struct {
	ModelCount    int64 `json:"model_count"`
	InstanceCount int64 `json:"instance_count"`
	BusinessCount int64 `json:"business_count"`
	HostCount     int64 `json:"host_count"`
}

func (s *StatsService) Overview() (*Overview, error) {
	o := &Overview{}
	// 仅统计启用模型及其实例：停用模型不计入资产总览
	s.db.Model(&model.Model{}).Where("enabled = ?", true).Count(&o.ModelCount)
	s.db.Model(&model.Instance{}).
		Joins("JOIN model ON model.id = instance.model_id").
		Where("model.enabled = ?", true).Count(&o.InstanceCount)

	// 业务数 = biz 模型实例数（与资源实例、拓扑同源）
	var biz model.Model
	if err := s.db.Where("obj_id = ?", "biz").First(&biz).Error; err == nil {
		s.db.Model(&model.Instance{}).Where("model_id = ?", biz.ID).Count(&o.BusinessCount)
	}

	var host model.Model
	if err := s.db.Where("obj_id = ?", "host").First(&host).Error; err == nil {
		s.db.Model(&model.Instance{}).Where("model_id = ?", host.ID).Count(&o.HostCount)
	}
	return o, nil
}

// NameCount 名称-数量对，用于图表聚合数据。
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ModelCount 带模型 id 的数量项（供前端点击跳转到对应模型）。
type ModelCount struct {
	ModelID uint   `json:"model_id"`
	Name    string `json:"name"`
	Count   int64  `json:"count"`
}

// Analysis 运营分析看板数据。
type Analysis struct {
	Overview        Overview    `json:"overview"`
	ModelInstances  []ModelCount `json:"model_instances"`  // 各模型实例数（含 model_id 便于跳转）
	HostOS          []NameCount `json:"host_os"`          // 主机操作系统分布
	HostStatus      []NameCount `json:"host_status"`      // 主机状态分布
	BizHosts        []NameCount `json:"biz_hosts"`        // 各业务主机数（拓扑归属）
	BizHostsAll     []NameCount `json:"biz_hosts_all"`    // 各业务关联主机数（归属 + 服务关联，去重）
	Unassigned      []ModelCount `json:"unassigned"`       // 未归属资源数（未分配到业务拓扑），按模型
	UnassignedTotal int64        `json:"unassigned_total"` // 未归属资源总数
}

func (s *StatsService) Analysis() (*Analysis, error) {
	ov, err := s.Overview()
	if err != nil {
		return nil, err
	}
	a := &Analysis{Overview: *ov, ModelInstances: []ModelCount{}, HostOS: []NameCount{}, HostStatus: []NameCount{}, BizHosts: []NameCount{}, BizHostsAll: []NameCount{}, Unassigned: []ModelCount{}}

	// 各模型实例数（带 model_id，供前端点击跳转到对应模型）；停用模型不计入资产总览
	s.db.Model(&model.Instance{}).
		Select("instance.model_id as model_id, model.name as name, count(instance.id) as count").
		Joins("JOIN model ON model.id = instance.model_id").
		Where("model.enabled = ?", true).
		Group("instance.model_id").
		Order("count desc").
		Scan(&a.ModelInstances)

	// 主机操作系统分布
	var host model.Model
	if err := s.db.Where("obj_id = ?", "host").First(&host).Error; err == nil {
		s.db.Model(&model.Instance{}).
			Select("COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_os_name')), ''), '未知') as name, count(*) as count").
			Where("model_id = ?", host.ID).
			Group("name").
			Order("count desc").
			Scan(&a.HostOS)

		// 主机状态分布（状态存选项 id，映射为选项名称展示）
		a.HostStatus = s.hostStatusDist(host.ID)
	}

	// 各业务主机数（拓扑归属中模型为主机的成员，业务名取自 biz 实例）
	if host.ID != 0 {
		s.db.Table("host_module AS hm").
			Select("JSON_UNQUOTE(JSON_EXTRACT(b.data, '$.bk_biz_name')) as name, count(*) as count").
			Joins("JOIN instance b ON b.id = hm.biz_id").
			Joins("JOIN instance h ON h.id = hm.host_id").
			Where("h.model_id = ?", host.ID).
			Group("hm.biz_id").
			Order("count desc").
			Scan(&a.BizHosts)
	}

	// 各业务关联主机数（含共享）= 拓扑归属 ∪ 「服务于」关联，按业务去重
	a.BizHostsAll = s.bizHostsAll()

	// 未归属资源：未填「业务线」字段(data.biz_line 空/null/0)的实例，按模型统计（排除 业务/集群/模块 本身）
	bizExpr := "JSON_UNQUOTE(JSON_EXTRACT(instance.data, '$.biz_line'))"
	s.db.Model(&model.Instance{}).
		Select("instance.model_id as model_id, model.name as name, count(instance.id) as count").
		Joins("JOIN model ON model.id = instance.model_id").
		Where("model.enabled = ?", true).
		Where("model.obj_id NOT IN ?", []string{"biz", "set", "module"}).
		Where("(" + bizExpr + " IS NULL OR " + bizExpr + " IN ('', 'null', '0'))").
		Group("instance.model_id").
		Order("count desc").
		Scan(&a.Unassigned)
	for _, u := range a.Unassigned {
		a.UnassignedTotal += u.Count
	}

	return a, nil
}

// bizHostsAll 按资源的「业务线」字段(data.biz_line=业务实例id)统计各业务线下的资源数
// （排除 biz/set/module 本身；与拓扑 host_module 解耦，口径同「归属」筛选）。
func (s *StatsService) bizHostsAll() []NameCount {
	bizExpr := "JSON_UNQUOTE(JSON_EXTRACT(instance.data, '$.biz_line'))"
	type bc struct {
		Bid   string
		Count int64
	}
	var rows []bc
	s.db.Model(&model.Instance{}).
		Select(bizExpr+" as bid, count(*) as count").
		Joins("JOIN model ON model.id = instance.model_id").
		Where("model.enabled = ?", true).
		Where("model.obj_id NOT IN ?", []string{"biz", "set", "module"}).
		Where("(" + bizExpr + " IS NOT NULL AND " + bizExpr + " NOT IN ('', 'null', '0'))").
		Group("bid").Scan(&rows)

	// 业务线 id → 名称（仅保留仍存在的业务实例）
	nameMap := map[string]string{}
	var bizModel model.Model
	if err := s.db.Where("obj_id = ?", "biz").First(&bizModel).Error; err == nil {
		var insts []model.Instance
		s.db.Where("model_id = ?", bizModel.ID).Find(&insts)
		for _, i := range insts {
			nameMap[strconv.FormatUint(uint64(i.ID), 10)] = bizName(i.Data)
		}
	}

	out := make([]NameCount, 0, len(rows))
	for _, r := range rows {
		name := nameMap[r.Bid]
		if name == "" {
			continue // 业务线已删除的孤儿值，跳过
		}
		out = append(out, NameCount{Name: name, Count: r.Count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}
