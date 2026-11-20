package model

import (
	"time"

	"gorm.io/datatypes"
)

// AuditLog 操作审计日志，记录所有写操作。
type AuditLog struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"size:64;index" json:"username"`
	Action       string         `gorm:"size:16" json:"action"`              // HTTP 方法
	ActionText   string         `gorm:"size:32" json:"action_text"`         // 动作（中文：新增/编辑/删除/分配…）
	Object       string         `gorm:"size:64;index" json:"object"`        // 操作对象（中文：主机/模型/业务/角色…）
	Instance     string         `gorm:"size:255" json:"instance"`           // 具体实例/目标展示
	BizName      string         `gorm:"size:128" json:"biz_name"`           // 所属业务
	Summary      string         `gorm:"size:512" json:"summary"`            // 操作描述
	ResourceType string         `gorm:"size:64;index" json:"resource_type"` // 资源类型（路径首段，原始）
	ResourcePath string         `gorm:"size:255" json:"resource_path"`
	Detail       datatypes.JSON `gorm:"type:json" json:"detail"` // 请求体摘要
	Status       int            `gorm:"index" json:"status"`     // 响应状态码
	IP           string         `gorm:"size:64" json:"ip"`
	CreatedAt    time.Time      `gorm:"index" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_log" }
