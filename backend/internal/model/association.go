package model

import "time"

// 关联基数（映射关系）
const (
	MappingOneToOne   = "1:1"
	MappingOneToMany  = "1:n"
	MappingManyToMany = "n:n"
)

// ValidMappings 校验基数合法性。
var ValidMappings = map[string]bool{
	MappingOneToOne: true, MappingOneToMany: true, MappingManyToMany: true,
}

// AssociationKind 关联类型（关系语义），如 belong/run/connect。
type AssociationKind struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Code         string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name         string    `gorm:"size:64" json:"name"`
	SrcToDstNote string    `gorm:"size:64" json:"src_to_dst_note"` // 源到目标描述
	DstToSrcNote string    `gorm:"size:64" json:"dst_to_src_note"` // 目标到源描述
	IsBuiltin    bool      `gorm:"default:false" json:"is_builtin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AssociationKind) TableName() string { return "association_kind" }

// ModelAssociation 模型间关联定义。
type ModelAssociation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Code       string    `gorm:"size:128;uniqueIndex;not null" json:"code"`
	KindID     uint      `gorm:"not null" json:"kind_id"`
	SrcModelID uint      `gorm:"index;not null" json:"src_model_id"`
	DstModelID uint      `gorm:"index;not null" json:"dst_model_id"`
	Mapping    string    `gorm:"size:8" json:"mapping"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ModelAssociation) TableName() string { return "model_association" }
