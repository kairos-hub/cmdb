package model

import "time"

// 拓扑节点类型
const (
	TopoBiz      = "biz"      // 业务线（存为 biz 模型实例，非 TopoNode）
	TopoBusiness = "business" // 业务（业务线下可有多个，各自独立一张拓扑画布）
	TopoSet      = "set"      // 集群 / 环境（已废弃，保留常量供历史迁移）
	TopoModule   = "module"   // 模块（模型节点）
)

// TopoNode 业务拓扑节点，一张表表达 业务线→业务→模块 三层树。
//   - 业务线：存为 biz 模型实例（非 TopoNode），其 id 作为各节点的 biz_id
//   - 业务：node_type=business, parent_id=业务线id, biz_id=业务线id
//   - 模块：node_type=module, parent_id=业务节点id, biz_id=业务线id, model_id=承载的资源类型,
//     pos_x/pos_y=自由画布中的坐标
//
// 注：集群层(set)已去除，模块直接挂业务；模块间连线由用户自建，存 TopoEdge。
type TopoNode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	BizID     uint      `gorm:"index;not null" json:"biz_id"`
	ParentID  uint      `gorm:"index;not null" json:"parent_id"`
	NodeType  string    `gorm:"size:16;not null" json:"node_type"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	ModelID   uint      `gorm:"default:0" json:"model_id"` // 仅模块使用：该模块承载的资源模型（0=未指定/兼容旧数据按主机）
	PosX      int       `gorm:"default:0" json:"pos_x"`    // 自由画布坐标
	PosY      int       `gorm:"default:0" json:"pos_y"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TopoNode) TableName() string { return "topo_node" }

// TopoEdge 用户在业务画布上自建的有向连线，起点可以是所在业务节点。
type TopoEdge struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BizID       uint      `gorm:"index;not null" json:"biz_id"`      // 业务线
	BusinessID  uint      `gorm:"index;not null" json:"business_id"` // 所属业务节点
	SrcModuleID uint      `gorm:"index;not null" json:"src_module_id"`
	DstModuleID uint      `gorm:"index;not null" json:"dst_module_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (TopoEdge) TableName() string { return "topo_edge" }

// HostModule 资源实例与模块的归属关系（一个实例至多属于一个模块；无记录即在资源池）。
// 历史名为主机归属，现已泛化为任意资源类型（host_id 存资源实例 id）。
type HostModule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	HostID    uint      `gorm:"uniqueIndex;not null" json:"host_id"` // 资源实例 id
	ModuleID  uint      `gorm:"index;not null" json:"module_id"`     // 模块节点 id
	ModelID   uint      `gorm:"index;default:0" json:"model_id"`     // 资源模型 id
	BizID     uint      `gorm:"index;not null" json:"biz_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (HostModule) TableName() string { return "host_module" }
