package model

import "time"

// ModelGroup 模型分组，用于对模型分类管理。
type ModelGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Index     int       `gorm:"default:0" json:"index"`
	IsBuiltin bool      `gorm:"default:false" json:"is_builtin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ModelGroup) TableName() string { return "model_group" }
