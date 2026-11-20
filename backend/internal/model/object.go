package model

import "time"

// Model 模型（对象）定义，CMDB 中各类资源类型的元数据。
type Model struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GroupID   uint      `gorm:"index;not null" json:"group_id"`
	ObjID     string    `gorm:"size:64;uniqueIndex;not null" json:"obj_id"` // 模型唯一标识
	Name      string    `gorm:"size:64;not null" json:"name"`
	Icon      string    `gorm:"size:64" json:"icon"`
	Index     int       `gorm:"default:0" json:"index"` // 分组内展示排序，越小越靠前
	IsBuiltin bool      `gorm:"default:false" json:"is_builtin"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Model) TableName() string { return "model" }
