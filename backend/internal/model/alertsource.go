package model

import "time"

// 告警源类型（与数据源解耦，独立于资源同步）
const (
	AlertSourceAliyun  = "aliyun"  // 阿里云云监控：拉取报警历史
	AlertSourceArms    = "arms"    // 阿里云 ARMS 告警管理：拉取 ACK 集群告警（ListAlerts）
	AlertSourceTencent = "tencent" // 腾讯云云监控：拉取报警历史
	AlertSourceWebhook = "webhook" // 通用告警回调：夜莺/Alertmanager/Grafana/自定义 HTTP 推送
	AlertSourcePrometheus = "prometheus" // 自建 Prometheus：主动拉取 /api/v1/alerts（含 K8s 应用级告警）
)

// AlertSource 告警源配置：专用于告警接入（拉取或回调），与 DataSource（资源同步）彻底分离。
type AlertSource struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:64;not null" json:"name"`
	Type           string     `gorm:"size:32;not null;default:'webhook'" json:"type"` // aliyun/tencent/webhook
	Region         string     `gorm:"size:64" json:"region"`                          // 云地域（阿里云/腾讯云），可逗号分隔多地域取首个
	URL            string     `gorm:"size:255" json:"url"`                            // Prometheus 地址（prometheus 类型用，拉 /api/v1/alerts）
	Username       string     `gorm:"size:128" json:"username"`                       // 阿里云 AccessKeyId / 腾讯云 SecretId
	Password       string     `gorm:"size:512" json:"password"`                       // 阿里云 AccessKeySecret / 腾讯云 SecretKey
	Token          string     `gorm:"size:512" json:"token"`                          // webhook 校验 token（创建时自动生成）
	AlertCron      string     `gorm:"size:64" json:"alert_cron"`                      // 告警拉取 cron（5 段），空=不自动；webhook 不用
	AlertWindowMin int        `gorm:"default:0" json:"alert_window_min"`              // 每次拉取时间窗（分钟），0=默认20；上限15天
	LastAlertAt    *time.Time `json:"last_alert_at"`                                  // 上次告警拉取/接收时间
	LastAlertMsg   string     `gorm:"size:255" json:"last_alert_msg"`                 // 上次结果
	Enabled        bool       `gorm:"not null;default:true" json:"enabled"`
	SortOrder      int        `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (AlertSource) TableName() string { return "alert_source" }
