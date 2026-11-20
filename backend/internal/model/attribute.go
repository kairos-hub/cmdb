package model

import (
	"time"

	"gorm.io/datatypes"
)

// 字段类型
const (
	FieldSingleChar = "singlechar" // 单行文本
	FieldLongChar   = "longchar"   // 长文本
	FieldInt        = "int"        // 整数
	FieldFloat      = "float"      // 浮点数
	FieldBool       = "bool"       // 布尔
	FieldEnum       = "enum"       // 枚举单选
	FieldEnumMulti  = "enummulti"  // 枚举多选
	FieldDate       = "date"       // 日期
	FieldDateTime   = "datetime"   // 日期时间
	FieldUser       = "user"       // 用户
	FieldList       = "list"       // 列表
)

// ValidFieldTypes 用于校验字段类型合法性。
var ValidFieldTypes = map[string]bool{
	FieldSingleChar: true, FieldLongChar: true, FieldInt: true, FieldFloat: true,
	FieldBool: true, FieldEnum: true, FieldEnumMulti: true, FieldDate: true,
	FieldDateTime: true, FieldUser: true, FieldList: true,
}

// Attribute 模型字段（属性）定义。
type Attribute struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	ModelID    uint           `gorm:"not null;uniqueIndex:idx_model_prop" json:"model_id"`
	PropertyID string         `gorm:"size:64;not null;uniqueIndex:idx_model_prop" json:"property_id"` // 模型内唯一
	Name       string         `gorm:"size:64;not null" json:"name"`
	Type       string         `gorm:"size:32;not null" json:"type"`
	Required   bool           `gorm:"default:false" json:"required"`
	Unique     bool           `gorm:"default:false" json:"unique"`
	Editable   bool           `gorm:"default:true" json:"editable"`
	Enabled    bool           `gorm:"not null;default:true" json:"enabled"`     // 是否启用（停用后在实例表单/列表/导出中隐藏）
	ListHidden bool           `gorm:"not null;default:false" json:"list_hidden"` // 列表隐藏：字段仍可用，但不在资源实例列表展示列
	Options    datatypes.JSON `gorm:"type:json" json:"options"` // 枚举选项等
	Default    string         `gorm:"size:255" json:"default"`
	Regex      string         `gorm:"size:255" json:"regex"`
	Group      string         `gorm:"size:64;default:'default'" json:"group"` // 字段分栏
	Index      int            `gorm:"default:0" json:"index"`
	IsBuiltin  bool           `gorm:"default:false" json:"is_builtin"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (Attribute) TableName() string { return "attribute" }
