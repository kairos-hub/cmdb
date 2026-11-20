package model

import (
	"time"

	"gorm.io/datatypes"
)

// 权限操作
const (
	ActionView   = "view"
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// 功能权限（菜单 / 管理操作级，独立于按模型的 CRUD）。
// 每个管理模块分「只读(view)」与「编辑(manage)」两档；编辑自动含只读（见 FuncImplies）。
const (
	FuncModelManage    = "model_manage"      // 模型管理-编辑（分组/模型/属性/关联增删改）
	FuncModelView      = "model_view"        // 模型管理-只读（仅查看结构）
	FuncTopoManage     = "topo_manage"       // 业务拓扑-编辑（业务/集群/模块/资源分配）
	FuncTopoView       = "topo_view"         // 业务拓扑-只读
	FuncUserManage     = "user_manage"       // 用户与角色-编辑
	FuncUserView       = "user_view"         // 用户与角色-只读
	FuncDataSource     = "datasource_manage" // 数据源-编辑（配置与同步）
	FuncDataSourceView = "datasource_view"   // 数据源-只读
	FuncStatsView      = "stats_view"        // 运营分析（本身只读）
	FuncAuditView      = "audit_view"        // 审计日志（本身只读）
)

// AllFunctions 全部功能权限码（超级管理员恒拥有全部）。
var AllFunctions = []string{
	FuncModelManage, FuncModelView,
	FuncTopoManage, FuncTopoView,
	FuncUserManage, FuncUserView,
	FuncDataSource, FuncDataSourceView,
	FuncStatsView, FuncAuditView,
}

// FuncImplies 定义「编辑」功能自动蕴含的「只读」功能（有编辑必能查看）。
// 聚合用户有效功能时据此展开，故只需授予编辑即同时拥有只读。
var FuncImplies = map[string]string{
	FuncModelManage: FuncModelView,
	FuncTopoManage:  FuncTopoView,
	FuncUserManage:  FuncUserView,
	FuncDataSource:  FuncDataSourceView,
}

// Role 角色。IsAdmin 为超级管理员，拥有全部权限。
type Role struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Code        string         `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name        string         `gorm:"size:64;not null" json:"name"`
	Description string         `gorm:"size:255" json:"description"`
	IsAdmin     bool           `gorm:"default:false" json:"is_admin"`
	IsBuiltin   bool           `gorm:"default:false" json:"is_builtin"`
	Functions   datatypes.JSON `gorm:"type:json" json:"functions"` // 功能权限码数组，如 ["model_manage"]
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (Role) TableName() string { return "role" }

// RolePermission 角色对某模型的操作权限。ModelID=0 表示全部模型。
type RolePermission struct {
	ID      uint           `gorm:"primaryKey" json:"id"`
	RoleID  uint           `gorm:"index;not null" json:"role_id"`
	ModelID uint           `gorm:"not null" json:"model_id"`
	Actions datatypes.JSON `gorm:"type:json" json:"actions"` // ["view","create","update","delete"]
}

func (RolePermission) TableName() string { return "role_permission" }

// UserRole 用户与角色的多对多关系。
type UserRole struct {
	UserID uint `gorm:"primaryKey" json:"user_id"`
	RoleID uint `gorm:"primaryKey" json:"role_id"`
}

func (UserRole) TableName() string { return "user_role" }
