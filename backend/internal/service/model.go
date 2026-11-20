package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

var (
	ErrNotFound         = errors.New("记录不存在")
	ErrDuplicate        = errors.New("标识已存在")
	ErrBuiltinProtected = errors.New("内置对象受保护，不可删除")
	ErrInvalidFieldType = errors.New("非法的字段类型")
	ErrInvalidMapping   = errors.New("非法的关联基数")
	ErrGroupNotEmpty    = errors.New("分组下存在模型，无法删除")
	ErrInvalidRef       = errors.New("关联对象不存在")
	ErrReferenced       = errors.New("对象被引用，无法删除")
)

// ModelService 模型管理业务逻辑：分组、模型、字段、关联。
type ModelService struct {
	db *gorm.DB
}

func NewModelService(db *gorm.DB) *ModelService {
	return &ModelService{db: db}
}

func (s *ModelService) exists(dst interface{}, id uint) bool {
	var cnt int64
	s.db.Model(dst).Where("id = ?", id).Count(&cnt)
	return cnt > 0
}

// ---------------- 模型分组 ----------------

func (s *ModelService) ListGroups() ([]model.ModelGroup, error) {
	var groups []model.ModelGroup
	err := s.db.Order("`index` asc, id asc").Find(&groups).Error
	return groups, err
}

func (s *ModelService) CreateGroup(g *model.ModelGroup) error {
	var cnt int64
	s.db.Model(&model.ModelGroup{}).Where("code = ?", g.Code).Count(&cnt)
	if cnt > 0 {
		return ErrDuplicate
	}
	g.IsBuiltin = false
	return s.db.Create(g).Error
}

func (s *ModelService) UpdateGroup(id uint, name string, index int) (*model.ModelGroup, error) {
	var g model.ModelGroup
	if err := s.db.First(&g, id).Error; err != nil {
		return nil, ErrNotFound
	}
	g.Name = name
	g.Index = index
	if err := s.db.Save(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *ModelService) DeleteGroup(id uint) error {
	var g model.ModelGroup
	if err := s.db.First(&g, id).Error; err != nil {
		return ErrNotFound
	}
	if g.IsBuiltin {
		return ErrBuiltinProtected
	}
	var cnt int64
	s.db.Model(&model.Model{}).Where("group_id = ?", id).Count(&cnt)
	if cnt > 0 {
		return ErrGroupNotEmpty
	}
	return s.db.Delete(&g).Error
}

// ---------------- 模型 ----------------

func (s *ModelService) ListModels(groupID uint) ([]model.Model, error) {
	// 默认按名称字母顺序；已手工排序(index>0)的按 index 优先
	q := s.db.Order("`index` asc, name asc")
	if groupID > 0 {
		q = q.Where("group_id = ?", groupID)
	}
	var models []model.Model
	err := q.Find(&models).Error
	return models, err
}

// ReorderModels 按给定顺序重排某分组内模型（index 从 1 递增）。
func (s *ModelService) ReorderModels(groupID uint, orderedIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range orderedIDs {
			if err := tx.Model(&model.Model{}).
				Where("id = ? AND group_id = ?", id, groupID).
				Update("index", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ModelService) GetModel(id uint) (*model.Model, error) {
	var m model.Model
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, ErrNotFound
	}
	return &m, nil
}

func (s *ModelService) CreateModel(m *model.Model) error {
	if !s.exists(&model.ModelGroup{}, m.GroupID) {
		return ErrInvalidRef
	}
	var cnt int64
	s.db.Model(&model.Model{}).Where("obj_id = ?", m.ObjID).Count(&cnt)
	if cnt > 0 {
		return ErrDuplicate
	}
	m.IsBuiltin = false
	m.Enabled = true
	// 已手工排序的分组(存在 index>0)：新模型排到末尾；未排序分组：保持 0，按名称字母序融入
	var maxIdx int
	s.db.Model(&model.Model{}).Where("group_id = ?", m.GroupID).
		Select("COALESCE(MAX(`index`), 0)").Scan(&maxIdx)
	if maxIdx > 0 {
		m.Index = maxIdx + 1
	}
	return s.db.Create(m).Error
}

func (s *ModelService) UpdateModel(id uint, name, icon string, groupID uint, enabled bool) (*model.Model, error) {
	var m model.Model
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, ErrNotFound
	}
	if groupID > 0 && groupID != m.GroupID {
		if !s.exists(&model.ModelGroup{}, groupID) {
			return nil, ErrInvalidRef
		}
		m.GroupID = groupID
	}
	m.Name = name
	m.Icon = icon
	m.Enabled = enabled
	if err := s.db.Save(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *ModelService) DeleteModel(id uint) error {
	var m model.Model
	if err := s.db.First(&m, id).Error; err != nil {
		return ErrNotFound
	}
	if m.IsBuiltin {
		return ErrBuiltinProtected
	}
	// 级联删除字段
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model_id = ?", id).Delete(&model.Attribute{}).Error; err != nil {
			return err
		}
		return tx.Delete(&m).Error
	})
}

// ---------------- 字段 ----------------

func (s *ModelService) ListAttributes(modelID uint) ([]model.Attribute, error) {
	var attrs []model.Attribute
	err := s.db.Where("model_id = ?", modelID).Order("`index` asc, id asc").Find(&attrs).Error
	return attrs, err
}

func (s *ModelService) CreateAttribute(a *model.Attribute) error {
	if !model.ValidFieldTypes[a.Type] {
		return ErrInvalidFieldType
	}
	if !s.exists(&model.Model{}, a.ModelID) {
		return ErrInvalidRef
	}
	var cnt int64
	s.db.Model(&model.Attribute{}).Where("model_id = ? AND property_id = ?", a.ModelID, a.PropertyID).Count(&cnt)
	if cnt > 0 {
		return ErrDuplicate
	}
	a.IsBuiltin = false
	if a.Group == "" {
		a.Group = "default"
	}
	// 默认追加到该模型字段末尾
	var maxIndex int
	s.db.Model(&model.Attribute{}).Where("model_id = ?", a.ModelID).
		Select("COALESCE(MAX(`index`), 0)").Scan(&maxIndex)
	a.Index = maxIndex + 1
	return s.db.Create(a).Error
}

// ReorderAttributes 按给定顺序重排模型字段（index 从 1 递增）。
func (s *ModelService) ReorderAttributes(modelID uint, orderedIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range orderedIDs {
			if err := tx.Model(&model.Attribute{}).
				Where("id = ? AND model_id = ?", id, modelID).
				Updates(map[string]interface{}{"index": i + 1}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateAttribute 更新字段。内置字段不允许改类型与标识，仅可改展示类属性。
// GetAttribute 按 id 取字段（供审计等场景拿到字段显示名与所属模型）。
func (s *ModelService) GetAttribute(id uint) (*model.Attribute, error) {
	var a model.Attribute
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, ErrNotFound
	}
	return &a, nil
}

// UpdateAttribute 更新字段，返回更新后的字段与变更明细（供审计记录「改了什么」）。
func (s *ModelService) UpdateAttribute(id uint, in *model.Attribute) (*model.Attribute, []string, error) {
	var a model.Attribute
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, nil, ErrNotFound
	}
	if in.Type != "" && !model.ValidFieldTypes[in.Type] {
		return nil, nil, ErrInvalidFieldType
	}
	var changes []string
	addStr := func(label, ov, nv string) {
		if ov != nv {
			changes = append(changes, fmt.Sprintf("%s: %s → %s", label, dash(ov), dash(nv)))
		}
	}
	addBool := func(label string, ov, nv bool) {
		if ov != nv {
			changes = append(changes, fmt.Sprintf("%s: %s → %s", label, boolCN(ov), boolCN(nv)))
		}
	}
	addStr("显示名", a.Name, in.Name)
	addBool("必填", a.Required, in.Required)
	addBool("唯一", a.Unique, in.Unique)
	addBool("可编辑", a.Editable, in.Editable)
	addBool("启用", a.Enabled, in.Enabled)
	addBool("列表隐藏", a.ListHidden, in.ListHidden)
	addStr("默认值", a.Default, in.Default)
	addStr("正则", a.Regex, in.Regex)
	if in.Group != "" {
		addStr("分组", a.Group, in.Group)
	}
	if a.Index != in.Index {
		changes = append(changes, fmt.Sprintf("排序: %d → %d", a.Index, in.Index))
	}
	if !a.IsBuiltin && in.Type != "" {
		addStr("类型", a.Type, in.Type)
	}
	if o, n := strings.TrimSpace(string(a.Options)), strings.TrimSpace(string(in.Options)); o != n && (o != "" || n != "") {
		changes = append(changes, "可选项: 已调整")
	}

	a.Name = in.Name
	a.Required = in.Required
	a.Unique = in.Unique
	a.Editable = in.Editable
	a.Enabled = in.Enabled
	a.ListHidden = in.ListHidden
	a.Options = in.Options
	a.Default = in.Default
	a.Regex = in.Regex
	if in.Group != "" {
		a.Group = in.Group
	}
	a.Index = in.Index
	if !a.IsBuiltin && in.Type != "" {
		a.Type = in.Type
	}
	if err := s.db.Save(&a).Error; err != nil {
		return nil, nil, err
	}
	return &a, changes, nil
}

func (s *ModelService) DeleteAttribute(id uint) error {
	var a model.Attribute
	if err := s.db.First(&a, id).Error; err != nil {
		return ErrNotFound
	}
	if a.IsBuiltin {
		return ErrBuiltinProtected
	}
	return s.db.Delete(&a).Error
}

// dash 空串显示为「-」。
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// boolCN 布尔值中文化。
func boolCN(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

// ---------------- 关联类型 ----------------

func (s *ModelService) ListKinds() ([]model.AssociationKind, error) {
	var kinds []model.AssociationKind
	err := s.db.Order("id asc").Find(&kinds).Error
	return kinds, err
}

func (s *ModelService) CreateKind(k *model.AssociationKind) error {
	var cnt int64
	s.db.Model(&model.AssociationKind{}).Where("code = ?", k.Code).Count(&cnt)
	if cnt > 0 {
		return ErrDuplicate
	}
	k.IsBuiltin = false
	return s.db.Create(k).Error
}

func (s *ModelService) DeleteKind(id uint) error {
	var k model.AssociationKind
	if err := s.db.First(&k, id).Error; err != nil {
		return ErrNotFound
	}
	if k.IsBuiltin {
		return ErrBuiltinProtected
	}
	var cnt int64
	s.db.Model(&model.ModelAssociation{}).Where("kind_id = ?", id).Count(&cnt)
	if cnt > 0 {
		return ErrReferenced
	}
	return s.db.Delete(&k).Error
}

// ---------------- 模型关联 ----------------

func (s *ModelService) ListAssociations() ([]model.ModelAssociation, error) {
	var assocs []model.ModelAssociation
	err := s.db.Order("id asc").Find(&assocs).Error
	return assocs, err
}

func (s *ModelService) CreateAssociation(a *model.ModelAssociation) error {
	if !model.ValidMappings[a.Mapping] {
		return ErrInvalidMapping
	}
	if !s.exists(&model.AssociationKind{}, a.KindID) ||
		!s.exists(&model.Model{}, a.SrcModelID) ||
		!s.exists(&model.Model{}, a.DstModelID) {
		return ErrInvalidRef
	}
	if a.Code == "" {
		a.Code = fmt.Sprintf("asst_%d_%d_%d", a.SrcModelID, a.KindID, a.DstModelID)
	}
	var cnt int64
	s.db.Model(&model.ModelAssociation{}).Where("code = ?", a.Code).Count(&cnt)
	if cnt > 0 {
		return ErrDuplicate
	}
	return s.db.Create(a).Error
}

func (s *ModelService) DeleteAssociation(id uint) error {
	var a model.ModelAssociation
	if err := s.db.First(&a, id).Error; err != nil {
		return ErrNotFound
	}
	return s.db.Delete(&a).Error
}
