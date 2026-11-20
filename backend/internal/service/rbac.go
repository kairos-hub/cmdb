package service

import (
	"encoding/json"
	"errors"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RBACService 角色、权限、用户角色与权限校验。
type RBACService struct {
	db *gorm.DB
}

func NewRBACService(db *gorm.DB) *RBACService {
	return &RBACService{db: db}
}

// ---------------- 角色 ----------------

func (s *RBACService) ListRoles() ([]model.Role, error) {
	var roles []model.Role
	err := s.db.Order("id asc").Find(&roles).Error
	return roles, err
}

func (s *RBACService) CreateRole(code, name, desc string, functions []string) (*model.Role, error) {
	var cnt int64
	s.db.Model(&model.Role{}).Where("code = ?", code).Count(&cnt)
	if cnt > 0 {
		return nil, ErrDuplicate
	}
	r := &model.Role{Code: code, Name: name, Description: desc, Functions: marshalFuncs(functions)}
	if err := s.db.Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

func (s *RBACService) UpdateRole(id uint, name, desc string, functions []string) (*model.Role, error) {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, ErrNotFound
	}
	r.Name = name
	r.Description = desc
	if !r.IsAdmin { // 超管功能权限恒为全部，不接受外部覆盖
		r.Functions = marshalFuncs(functions)
	}
	if err := s.db.Save(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// marshalFuncs 过滤非法功能码后序列化。
func marshalFuncs(functions []string) datatypes.JSON {
	valid := make([]string, 0, len(functions))
	for _, f := range functions {
		if contains(model.AllFunctions, f) && !contains(valid, f) {
			valid = append(valid, f)
		}
	}
	b, _ := json.Marshal(valid)
	return datatypes.JSON(b)
}

func (s *RBACService) DeleteRole(id uint) error {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		return ErrNotFound
	}
	if r.IsBuiltin {
		return ErrBuiltinProtected
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("role_id = ?", id).Delete(&model.RolePermission{})
		tx.Where("role_id = ?", id).Delete(&model.UserRole{})
		return tx.Delete(&r).Error
	})
}

// ---------------- 角色权限 ----------------

func (s *RBACService) GetRolePermissions(roleID uint) ([]model.RolePermission, error) {
	var perms []model.RolePermission
	err := s.db.Where("role_id = ?", roleID).Order("model_id asc").Find(&perms).Error
	return perms, err
}

// PermInput 单条权限配置。
type PermInput struct {
	ModelID uint     `json:"model_id"`
	Actions []string `json:"actions"`
}

// SetRolePermissions 覆盖式设置角色权限。
func (s *RBACService) SetRolePermissions(roleID uint, perms []PermInput) error {
	if _, err := s.getRole(roleID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		for _, p := range perms {
			if len(p.Actions) == 0 {
				continue
			}
			actions, _ := json.Marshal(p.Actions)
			if err := tx.Create(&model.RolePermission{
				RoleID: roleID, ModelID: p.ModelID, Actions: datatypes.JSON(actions),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *RBACService) getRole(id uint) (*model.Role, error) {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, ErrNotFound
	}
	return &r, nil
}

// ---------------- 用户角色 ----------------

func (s *RBACService) GetUserRoleIDs(userID uint) []uint {
	var ids []uint
	s.db.Model(&model.UserRole{}).Where("user_id = ?", userID).Pluck("role_id", &ids)
	return ids
}

// SetUserRoles 覆盖式设置用户角色。
func (s *RBACService) SetUserRoles(userID uint, roleIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		for _, rid := range roleIDs {
			if err := tx.Create(&model.UserRole{UserID: userID, RoleID: rid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------------- 权限校验 ----------------

func (s *RBACService) IsAdmin(userID uint) bool {
	var cnt int64
	s.db.Table("user_role").
		Joins("JOIN role ON role.id = user_role.role_id").
		Where("user_role.user_id = ? AND role.is_admin = ?", userID, true).
		Count(&cnt)
	return cnt > 0
}

// HasFunction 判断用户是否拥有某功能权限（超管恒为真）。
func (s *RBACService) HasFunction(userID uint, code string) bool {
	if s.IsAdmin(userID) {
		return true
	}
	return contains(s.GetUserFunctions(userID), code)
}

// GetUserFunctions 聚合用户所有角色的功能权限码（超管返回全部）。
func (s *RBACService) GetUserFunctions(userID uint) []string {
	if s.IsAdmin(userID) {
		return append([]string{}, model.AllFunctions...)
	}
	roleIDs := s.GetUserRoleIDs(userID)
	if len(roleIDs) == 0 {
		return []string{}
	}
	var roles []model.Role
	s.db.Where("id IN ?", roleIDs).Find(&roles)
	out := []string{}
	for _, r := range roles {
		var fns []string
		_ = json.Unmarshal(r.Functions, &fns)
		for _, f := range fns {
			if !contains(out, f) {
				out = append(out, f)
			}
		}
	}
	return expandFunctions(out)
}

// expandFunctions 按 FuncImplies 展开「编辑→只读」蕴含：拥有编辑功能即自动附带其只读功能。
func expandFunctions(fns []string) []string {
	out := append([]string{}, fns...)
	for _, f := range fns {
		if v, ok := model.FuncImplies[f]; ok && !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// HasPermission 判断用户对指定模型是否有某操作权限（超管恒为真）。
func (s *RBACService) HasPermission(userID, modelID uint, action string) bool {
	if s.IsAdmin(userID) {
		return true
	}
	roleIDs := s.GetUserRoleIDs(userID)
	if len(roleIDs) == 0 {
		return false
	}
	var perms []model.RolePermission
	s.db.Where("role_id IN ? AND model_id IN ?", roleIDs, []uint{0, modelID}).Find(&perms)
	for _, p := range perms {
		var actions []string
		_ = json.Unmarshal(p.Actions, &actions)
		if contains(actions, action) {
			return true
		}
	}
	return false
}

// UserPermissions 当前用户的有效权限视图。
type UserPermissions struct {
	IsAdmin     bool              `json:"is_admin"`
	Permissions map[uint][]string `json:"permissions"` // model_id -> actions（0 表示全部模型）
	Functions   []string          `json:"functions"`   // 功能权限码
}

func (s *RBACService) GetUserPermissions(userID uint) UserPermissions {
	if s.IsAdmin(userID) {
		return UserPermissions{IsAdmin: true, Permissions: map[uint][]string{}, Functions: append([]string{}, model.AllFunctions...)}
	}
	result := UserPermissions{IsAdmin: false, Permissions: map[uint][]string{}, Functions: s.GetUserFunctions(userID)}
	roleIDs := s.GetUserRoleIDs(userID)
	if len(roleIDs) == 0 {
		return result
	}
	var perms []model.RolePermission
	s.db.Where("role_id IN ?", roleIDs).Find(&perms)
	for _, p := range perms {
		var actions []string
		_ = json.Unmarshal(p.Actions, &actions)
		existing := result.Permissions[p.ModelID]
		for _, a := range actions {
			if !contains(existing, a) {
				existing = append(existing, a)
			}
		}
		result.Permissions[p.ModelID] = existing
	}
	return result
}

// EnsureAdminBinding 确保超管角色存在并绑定给指定用户名（幂等）。
func (s *RBACService) EnsureAdminBinding(username string) error {
	var role model.Role
	err := s.db.Where("code = ?", "admin").First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		role = model.Role{Code: "admin", Name: "超级管理员", Description: "拥有全部权限", IsAdmin: true, IsBuiltin: true}
		if err := s.db.Create(&role).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil // 用户不存在则跳过
	}
	var cnt int64
	s.db.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", user.ID, role.ID).Count(&cnt)
	if cnt == 0 {
		return s.db.Create(&model.UserRole{UserID: user.ID, RoleID: role.ID}).Error
	}
	return nil
}
