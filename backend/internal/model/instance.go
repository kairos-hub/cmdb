package model

import (
	"time"

	"gorm.io/datatypes"
)

// Instance 模型实例，动态属性统一存于 Data(JSON)。
// OfflineStatusID 失联/下线状态的统一选项 id（所有模型状态枚举共用）。
const OfflineStatusID = "offline"

type Instance struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	ModelID   uint           `gorm:"index;not null" json:"model_id"`
	Data      datatypes.JSON `gorm:"type:json" json:"data"`
	Creator   string         `gorm:"size:64" json:"creator"`
	Updater   string         `gorm:"size:64" json:"updater"`
	SourceID  uint           `gorm:"index;not null;default:0" json:"source_id"` // 来源数据源 id（0=手动/非同步）；按它做同源唯一与改名衔接
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	// BizLine 主机业务线：派生自业务拓扑归属（只读，不入库）
	BizLine string `gorm:"-" json:"biz_line,omitempty"`
	// SourceName/SourceType 来源数据源名与类型：creator 命中某数据源名时填充（只读）
	SourceName string `gorm:"-" json:"source_name,omitempty"`
	SourceType string `gorm:"-" json:"source_type,omitempty"`
}

func (Instance) TableName() string { return "instance" }

// InstanceRelation 实例间关联数据，建立在模型关联定义之上。
type InstanceRelation struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	AssociationID uint      `gorm:"index;not null" json:"association_id"`
	SrcInstID     uint      `gorm:"index;not null" json:"src_inst_id"`
	DstInstID     uint      `gorm:"index;not null" json:"dst_inst_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func (InstanceRelation) TableName() string { return "instance_relation" }
