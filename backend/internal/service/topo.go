package service

import (
	"encoding/json"
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SyncBizLineOptions 把所有资源模型「业务线」(biz_line) 枚举字段的选项刷新为当前业务线列表。
// 业务线新建/重命名/删除后调用，保证下拉选项与业务线一致。
// 选项 id=业务线实例 id（字符串），name=业务线名。
func SyncBizLineOptions(db *gorm.DB) error {
	var m model.Model
	if db.Where("obj_id = ?", "biz").First(&m).Error != nil || m.ID == 0 {
		return nil
	}
	var insts []model.Instance
	db.Where("model_id = ?", m.ID).Order("id asc").Find(&insts)
	type bizOpt struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	opts := make([]bizOpt, 0, len(insts))
	for _, i := range insts {
		opts = append(opts, bizOpt{ID: strconv.FormatUint(uint64(i.ID), 10), Name: bizName(i.Data)})
	}
	raw, _ := json.Marshal(opts)
	return db.Model(&model.Attribute{}).Where("property_id = ?", "biz_line").Update("options", datatypes.JSON(raw)).Error
}

// TopoService 业务拓扑与主机归属业务逻辑。
type TopoService struct {
	db *gorm.DB
}

func NewTopoService(db *gorm.DB) *TopoService {
	return &TopoService{db: db}
}

func (s *TopoService) hostModelID() uint {
	var m model.Model
	s.db.Where("obj_id = ?", "host").First(&m)
	return m.ID
}

func (s *TopoService) bizModelID() uint {
	var m model.Model
	s.db.Where("obj_id = ?", "biz").First(&m)
	return m.ID
}

// bizName 从 biz 实例数据中取业务名。
func bizName(raw []byte) string {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) == nil {
		if v, ok := m["bk_biz_name"].(string); ok {
			return v
		}
	}
	return ""
}

// bizExists 校验给定 id 是否为有效的业务实例。
func (s *TopoService) bizExists(id uint) bool {
	var cnt int64
	s.db.Model(&model.Instance{}).Where("id = ? AND model_id = ?", id, s.bizModelID()).Count(&cnt)
	return cnt > 0
}

func (s *TopoService) getNode(id uint, nodeType string) (*model.TopoNode, error) {
	var n model.TopoNode
	if err := s.db.Where("id = ? AND node_type = ?", id, nodeType).First(&n).Error; err != nil {
		return nil, ErrNotFound
	}
	return &n, nil
}

// ---------------- 业务 ----------------
// 业务统一以 biz 模型实例为唯一数据源；集群/模块仍为 TopoNode，biz_id 指向业务实例 id。

// BizNode 业务列表 / 创建返回的精简结构。
type BizNode struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`   // 负责人
	Contact string `json:"contact"` // 联系方式
}

// bizStr 从 biz 实例数据里取某个字符串字段。
func bizStr(raw []byte, key string) string {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) == nil {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

func (s *TopoService) ListBusiness() ([]BizNode, error) {
	var insts []model.Instance
	if err := s.db.Where("model_id = ?", s.bizModelID()).Order("id asc").Find(&insts).Error; err != nil {
		return nil, err
	}
	out := make([]BizNode, 0, len(insts))
	for _, i := range insts {
		out = append(out, BizNode{
			ID: i.ID, Name: bizName(i.Data),
			Owner: bizStr(i.Data, "owner"), Contact: bizStr(i.Data, "contact"),
		})
	}
	return out, nil
}

func (s *TopoService) CreateBiz(name, owner, contact string) (*BizNode, error) {
	bizModelID := s.bizModelID()
	if bizModelID == 0 {
		return nil, newValidationErr("业务模型不存在")
	}
	var cnt int64
	s.db.Model(&model.Instance{}).
		Where("model_id = ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_biz_name')) = ?", bizModelID, name).
		Count(&cnt)
	if cnt > 0 {
		return nil, newValidationErr("业务「%s」已存在", name)
	}
	raw, _ := json.Marshal(map[string]interface{}{"bk_biz_name": name, "owner": owner, "contact": contact})
	inst := model.Instance{ModelID: bizModelID, Data: datatypes.JSON(raw), Creator: "system", Updater: "system"}
	if err := s.db.Create(&inst).Error; err != nil {
		return nil, err
	}
	_ = SyncBizLineOptions(s.db)
	return &BizNode{ID: inst.ID, Name: name, Owner: owner, Contact: contact}, nil
}

func (s *TopoService) DeleteBiz(id uint) error {
	if !s.bizExists(id) {
		return ErrNotFound
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("biz_id = ?", id).Delete(&model.HostModule{}).Error; err != nil {
			return err
		}
		// 删除该业务下的业务/集群/模块节点
		if err := tx.Where("biz_id = ?", id).Delete(&model.TopoNode{}).Error; err != nil {
			return err
		}
		// 删除业务线实例本身
		return tx.Delete(&model.Instance{}, id).Error
	})
	if err == nil {
		_ = SyncBizLineOptions(s.db)
	}
	return err
}

// ---------------- 业务 ----------------
// 业务是业务线（biz 实例）下的一级，每个业务是一棵独立拓扑（集群→模块）。

// CreateBusiness 在业务线下新建业务节点。
func (s *TopoService) CreateBusiness(bizID uint, name string) (*model.TopoNode, error) {
	if !s.bizExists(bizID) {
		return nil, ErrInvalidRef
	}
	n := &model.TopoNode{NodeType: model.TopoBusiness, Name: name, BizID: bizID, ParentID: bizID}
	if err := s.db.Create(n).Error; err != nil {
		return nil, err
	}
	return n, nil
}

// DeleteBusiness 删除业务及其下全部模块、连线与资源归属（资源退回资源池）。
func (s *TopoService) DeleteBusiness(id uint) error {
	if _, err := s.getNode(id, model.TopoBusiness); err != nil {
		return err
	}
	// 该业务下的模块（现直属业务）
	var moduleIDs []uint
	s.db.Model(&model.TopoNode{}).Where("parent_id = ? AND node_type = ?", id, model.TopoModule).Pluck("id", &moduleIDs)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if len(moduleIDs) > 0 {
			if err := tx.Where("module_id IN ?", moduleIDs).Delete(&model.HostModule{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", moduleIDs).Delete(&model.TopoNode{}).Error; err != nil {
				return err
			}
		}
		// 删除该业务的连线
		if err := tx.Where("business_id = ?", id).Delete(&model.TopoEdge{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.TopoNode{}, id).Error
	})
}

// ---------------- 集群 ----------------

// CreateSet 在业务节点下新建集群。businessID 为业务节点 id。
func (s *TopoService) CreateSet(businessID uint, name string) (*model.TopoNode, error) {
	biz, err := s.getNode(businessID, model.TopoBusiness)
	if err != nil {
		return nil, ErrInvalidRef
	}
	n := &model.TopoNode{NodeType: model.TopoSet, Name: name, BizID: biz.BizID, ParentID: businessID}
	if err := s.db.Create(n).Error; err != nil {
		return nil, err
	}
	return n, nil
}

func (s *TopoService) DeleteSet(id uint) error {
	if _, err := s.getNode(id, model.TopoSet); err != nil {
		return err
	}
	var moduleIDs []uint
	s.db.Model(&model.TopoNode{}).Where("parent_id = ? AND node_type = ?", id, model.TopoModule).Pluck("id", &moduleIDs)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if len(moduleIDs) > 0 {
			if err := tx.Where("module_id IN ?", moduleIDs).Delete(&model.HostModule{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", moduleIDs).Delete(&model.TopoNode{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.TopoNode{}, id).Error
	})
}

// ---------------- 模块 ----------------

// CreateModule 在业务节点下新建模块（模型节点）。modelID 指定承载的资源类型，名称缺省取模型名，
// posX/posY 为自由画布坐标。
func (s *TopoService) CreateModule(businessID, modelID uint, name string, posX, posY int) (*model.TopoNode, error) {
	biz, err := s.getNode(businessID, model.TopoBusiness)
	if err != nil {
		return nil, ErrInvalidRef
	}
	var m model.Model
	if err := s.db.First(&m, modelID).Error; err != nil {
		return nil, newValidationErr("资源模型不存在")
	}
	if name == "" {
		name = m.Name
	}
	n := &model.TopoNode{NodeType: model.TopoModule, Name: name, BizID: biz.BizID, ParentID: businessID, ModelID: modelID, PosX: posX, PosY: posY}
	if err := s.db.Create(n).Error; err != nil {
		return nil, err
	}
	return n, nil
}

// UpdateModulePosition 保存模块/业务节点在自由画布中的坐标（业务锚点节点也可拖动摆放）。
func (s *TopoService) UpdateModulePosition(id uint, x, y int) error {
	var n model.TopoNode
	if err := s.db.Where("id = ? AND node_type IN ?", id, []string{model.TopoModule, model.TopoBusiness}).First(&n).Error; err != nil {
		return ErrNotFound
	}
	return s.db.Model(&model.TopoNode{}).Where("id = ?", id).Updates(map[string]interface{}{"pos_x": x, "pos_y": y}).Error
}

// ---------------- 连线（用户自建的模块间有向连接） ----------------

// ListEdges 返回某业务节点下的全部连线。
func (s *TopoService) ListEdges(businessID uint) ([]model.TopoEdge, error) {
	var edges []model.TopoEdge
	err := s.db.Where("business_id = ?", businessID).Order("id asc").Find(&edges).Error
	return edges, err
}

// CreateEdge 新建同一业务内的连线。起点可为业务节点或模块，终点必须为该业务下的模块。
func (s *TopoService) CreateEdge(businessID, src, dst uint) (*model.TopoEdge, error) {
	biz, err := s.getNode(businessID, model.TopoBusiness)
	if err != nil {
		return nil, ErrInvalidRef
	}
	if src == dst {
		return nil, newValidationErr("不能连接到自身")
	}
	for _, id := range []uint{src, dst} {
		if id == src && src == biz.ID {
			continue
		}
		var n model.TopoNode
		if s.db.Where("id = ? AND node_type = ? AND parent_id = ?", id, model.TopoModule, businessID).First(&n).Error != nil {
			return nil, newValidationErr("连线端点不属于该业务")
		}
	}
	// 去重：同方向已存在则直接返回
	var exist model.TopoEdge
	if s.db.Where("business_id = ? AND src_module_id = ? AND dst_module_id = ?", businessID, src, dst).First(&exist).Error == nil {
		return &exist, nil
	}
	e := &model.TopoEdge{BizID: biz.BizID, BusinessID: businessID, SrcModuleID: src, DstModuleID: dst}
	if err := s.db.Create(e).Error; err != nil {
		return nil, err
	}
	return e, nil
}

// DeleteEdge 删除连线。
func (s *TopoService) DeleteEdge(id uint) error {
	return s.db.Delete(&model.TopoEdge{}, id).Error
}

// moduleModelID 返回模块承载的资源模型（0 的旧数据按主机处理）。
func (s *TopoService) moduleModelID(mod *model.TopoNode) uint {
	if mod.ModelID != 0 {
		return mod.ModelID
	}
	return s.hostModelID()
}

func (s *TopoService) DeleteModule(id uint) error {
	if _, err := s.getNode(id, model.TopoModule); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("module_id = ?", id).Delete(&model.HostModule{}).Error; err != nil {
			return err
		}
		// 级联删除以该模块为端点的连线
		if err := tx.Where("src_module_id = ? OR dst_module_id = ?", id, id).Delete(&model.TopoEdge{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.TopoNode{}, id).Error
	})
}

// UpdateNodeName 重命名拓扑节点；业务重命名落到 biz 实例。
func (s *TopoService) UpdateNodeName(id uint, nodeType, name string) (interface{}, error) {
	if nodeType == model.TopoBiz {
		return s.renameBiz(id, name)
	}
	n, err := s.getNode(id, nodeType)
	if err != nil {
		return nil, err
	}
	n.Name = name
	if err := s.db.Model(n).Update("name", name).Error; err != nil {
		return nil, err
	}
	return n, nil
}

// renameBiz 仅改业务名（供通用 UpdateNodeName 调用，不动负责人/联系方式）。
func (s *TopoService) renameBiz(id uint, name string) (*BizNode, error) {
	return s.UpdateBiz(id, name, nil, nil)
}

// UpdateBiz 更新业务名（唯一性排除自身）及可选的负责人/联系方式。
// owner/contact 为 nil 表示不修改该字段（保留原值）；非 nil 则覆盖。
func (s *TopoService) UpdateBiz(id uint, name string, owner, contact *string) (*BizNode, error) {
	var inst model.Instance
	if err := s.db.Where("id = ? AND model_id = ?", id, s.bizModelID()).First(&inst).Error; err != nil {
		return nil, ErrNotFound
	}
	var cnt int64
	s.db.Model(&model.Instance{}).
		Where("model_id = ? AND id <> ? AND JSON_UNQUOTE(JSON_EXTRACT(data, '$.bk_biz_name')) = ?", s.bizModelID(), id, name).
		Count(&cnt)
	if cnt > 0 {
		return nil, newValidationErr("业务「%s」已存在", name)
	}
	var m map[string]interface{}
	if json.Unmarshal(inst.Data, &m) != nil || m == nil {
		m = map[string]interface{}{}
	}
	m["bk_biz_name"] = name
	if owner != nil {
		m["owner"] = *owner
	}
	if contact != nil {
		m["contact"] = *contact
	}
	raw, _ := json.Marshal(m)
	if err := s.db.Model(&inst).Update("data", datatypes.JSON(raw)).Error; err != nil {
		return nil, err
	}
	_ = SyncBizLineOptions(s.db) // 业务线改名后刷新各模型下拉选项的名称
	return &BizNode{ID: id, Name: name, Owner: bizStr(raw, "owner"), Contact: bizStr(raw, "contact")}, nil
}

// ---------------- 拓扑树 ----------------

// TopoTree 拓扑树节点。
type TopoTree struct {
	ID        uint        `json:"id"`
	Name      string      `json:"name"`
	Type      string      `json:"type"`
	HostCount int         `json:"host_count"`
	ModelID   uint        `json:"model_id,omitempty"`   // 模块承载的资源模型
	ModelName string      `json:"model_name,omitempty"` // 模块资源模型名
	PosX      int         `json:"pos_x"`                // 模块自由画布坐标
	PosY      int         `json:"pos_y"`
	Children  []*TopoTree `json:"children"`
}

// Tree 返回指定业务的完整拓扑树，含各节点主机数汇总。
func (s *TopoService) Tree(bizID uint) (*TopoTree, error) {
	var biz model.Instance
	if err := s.db.Where("id = ? AND model_id = ?", bizID, s.bizModelID()).First(&biz).Error; err != nil {
		return nil, ErrNotFound
	}

	var nodes []model.TopoNode
	s.db.Where("biz_id = ?", bizID).Order("id asc").Find(&nodes)

	type moduleCount struct {
		ModuleID uint
		Cnt      int
	}
	var counts []moduleCount
	s.db.Model(&model.HostModule{}).
		Select("module_id, count(*) as cnt").
		Where("biz_id = ? AND host_id IN (?)", bizID, s.db.Model(&model.Instance{}).Select("id")).
		Group("module_id").Scan(&counts)
	countMap := map[uint]int{}
	for _, c := range counts {
		countMap[c.ModuleID] = c.Cnt
	}

	// 按节点类型归类：业务线 → 业务 → 模块（集群层已去除，模块直属业务）
	var businesses []model.TopoNode
	modulesByBusiness := map[uint][]model.TopoNode{}
	for _, n := range nodes {
		switch n.NodeType {
		case model.TopoBusiness:
			businesses = append(businesses, n)
		case model.TopoModule:
			modulesByBusiness[n.ParentID] = append(modulesByBusiness[n.ParentID], n)
		}
	}

	// 模块资源模型名
	modelNames := map[uint]string{}
	{
		ids := map[uint]bool{}
		for _, n := range nodes {
			if n.NodeType == model.TopoModule && n.ModelID != 0 {
				ids[n.ModelID] = true
			}
		}
		if len(ids) > 0 {
			var ms []model.Model
			s.db.Select("id, name").Where("id IN ?", uintKeysT(ids)).Find(&ms)
			for _, m := range ms {
				modelNames[m.ID] = m.Name
			}
		}
	}

	root := &TopoTree{ID: biz.ID, Name: bizName(biz.Data), Type: model.TopoBiz, Children: []*TopoTree{}}
	for _, b := range businesses {
		bizTree := &TopoTree{ID: b.ID, Name: b.Name, Type: model.TopoBusiness, PosX: b.PosX, PosY: b.PosY, Children: []*TopoTree{}}
		for _, mod := range modulesByBusiness[b.ID] {
			cnt := countMap[mod.ID]
			bizTree.HostCount += cnt
			bizTree.Children = append(bizTree.Children, &TopoTree{
				ID: mod.ID, Name: mod.Name, Type: model.TopoModule,
				HostCount: cnt, ModelID: mod.ModelID, ModelName: modelNames[mod.ModelID],
				PosX: mod.PosX, PosY: mod.PosY, Children: []*TopoTree{},
			})
		}
		root.HostCount += bizTree.HostCount
		root.Children = append(root.Children, bizTree)
	}
	return root, nil
}

func uintKeysT(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------- 主机归属 ----------------

// IdleInstances 资源池：指定模型且未分配到任何模块的实例。modelID=0 时按主机模型。
func (s *TopoService) IdleInstances(modelID uint, page, pageSize int) (*ListResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 20
	}
	if modelID == 0 {
		modelID = s.hostModelID()
	}
	sub := s.db.Model(&model.HostModule{}).Select("host_id")
	q := s.db.Model(&model.Instance{}).
		Where("model_id = ?", modelID).
		Where("id NOT IN (?)", sub)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.Instance
	if err := q.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return &ListResult{List: list, Total: total, Page: page, PageSize: pageSize}, nil
}

// ModuleHosts 模块下的主机实例。
func (s *TopoService) ModuleHosts(moduleID uint) ([]model.Instance, error) {
	var hostIDs []uint
	s.db.Model(&model.HostModule{}).Where("module_id = ?", moduleID).Pluck("host_id", &hostIDs)
	if len(hostIDs) == 0 {
		return []model.Instance{}, nil
	}
	var hosts []model.Instance
	err := s.db.Where("id IN ?", hostIDs).Order("id desc").Find(&hosts).Error
	return hosts, err
}

// AssignHosts 将资源池实例分配到模块（实例模型须与模块承载的资源模型一致）。
func (s *TopoService) AssignHosts(moduleID uint, hostIDs []uint) error {
	mod, err := s.getNode(moduleID, model.TopoModule)
	if err != nil {
		return err
	}
	wantModel := s.moduleModelID(mod)
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, hid := range hostIDs {
			var inst model.Instance
			if err := tx.First(&inst, hid).Error; err != nil {
				return newValidationErr("资源 %d 不存在", hid)
			}
			if inst.ModelID != wantModel {
				return newValidationErr("资源 %d 与模块资源类型不一致", hid)
			}
			var cnt int64
			tx.Model(&model.HostModule{}).Where("host_id = ?", hid).Count(&cnt)
			if cnt > 0 {
				return newValidationErr("资源 %d 已被分配，请使用转移", hid)
			}
			if err := tx.Create(&model.HostModule{HostID: hid, ModuleID: moduleID, ModelID: inst.ModelID, BizID: mod.BizID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// TransferHosts 将已分配实例转移到目标模块（须同类型）。
func (s *TopoService) TransferHosts(targetModuleID uint, hostIDs []uint) error {
	mod, err := s.getNode(targetModuleID, model.TopoModule)
	if err != nil {
		return err
	}
	wantModel := s.moduleModelID(mod)
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, hid := range hostIDs {
			var hm model.HostModule
			if err := tx.Where("host_id = ?", hid).First(&hm).Error; err != nil {
				return newValidationErr("资源 %d 不在任何模块，请先分配", hid)
			}
			if hm.ModelID != 0 && hm.ModelID != wantModel {
				return newValidationErr("资源 %d 与目标模块资源类型不一致", hid)
			}
			if err := tx.Model(&model.HostModule{}).Where("host_id = ?", hid).
				Updates(map[string]interface{}{"module_id": targetModuleID, "biz_id": mod.BizID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RecycleHosts 将实例移回资源池（删除归属）。
func (s *TopoService) RecycleHosts(hostIDs []uint) (int64, error) {
	if len(hostIDs) == 0 {
		return 0, nil
	}
	res := s.db.Where("host_id IN ?", hostIDs).Delete(&model.HostModule{})
	return res.RowsAffected, res.Error
}
