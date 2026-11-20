package model

import "time"

// 打开方式
const (
	IntegrationEmbed = "embed" // 本系统内 iframe 内嵌打开
	IntegrationBlank = "blank" // 浏览器新标签页打开
)

// Integration 系统集成：在本系统入口打开的外部 Web 服务（Grafana、夜莺等）。
type Integration struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	URL         string    `gorm:"size:512;not null" json:"url"`
	Icon        string    `gorm:"size:64" json:"icon"`                       // Element 图标名或留空
	OpenMode    string    `gorm:"size:16;not null;default:'embed'" json:"open_mode"` // embed|blank
	Description string    `gorm:"size:255" json:"description"`
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Integration) TableName() string { return "integration" }
