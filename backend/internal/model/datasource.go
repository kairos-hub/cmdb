package model

import "time"

// 数据源类型
const (
	DataSourcePrometheus = "prometheus"
	DataSourceAliyun     = "aliyun"
	DataSourceTencent    = "tencent"
	DataSourceWebhook     = "webhook"     // 通用告警回调：任意平台 HTTP 推送告警，不做资源同步
	DataSourceNightingale = "nightingale" // 夜莺(n9e)：历史类型，等同 webhook（保留以兼容旧配置）
)

// DataSource 外部数据源配置（当前支持 Prometheus，用于资产发现）。
type DataSource struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Type      string    `gorm:"size:32;not null;default:'prometheus'" json:"type"`
	URL       string    `gorm:"size:255" json:"url"`                  // Prometheus 地址（阿里云不用）
	Query     string    `gorm:"size:255" json:"query"`                // PromQL，默认 up
	MonitorURL string   `gorm:"size:512" json:"monitor_url"`          // 「完整监控」跳转模板（Grafana等）；主机用 ${ip} ${host} ${id} ${region}、K8s应用用 ${namespace} ${workload} ${pod} ${cluster} ${app}，空则回退 Prometheus/云控制台
	Region    string    `gorm:"size:64" json:"region"`                // 阿里云地域，如 cn-hangzhou
	Username  string    `gorm:"size:128" json:"username"`             // Prometheus Basic Auth 账号 / 阿里云 AccessKeyId
	Password  string    `gorm:"size:512" json:"password"`             // Prometheus Basic Auth 密码 / 阿里云 AccessKeySecret
	Token     string    `gorm:"size:512" json:"token"`                // 可选 Bearer Token
	OnlyIP      bool       `gorm:"not null;default:true" json:"only_ip"` // 仅同步 instance 为合法 IP 的目标（过滤域名/URL 拨测）
	Cron        string     `gorm:"size:64" json:"cron"`                  // 资源同步 cron（5 段：分 时 日 月 周），空=不自动
	LastSyncAt  *time.Time `json:"last_sync_at"`                         // 上次资源同步时间
	LastSyncMsg string     `gorm:"size:255" json:"last_sync_msg"`        // 上次资源同步结果
	AlertCron      string   `gorm:"size:64" json:"alert_cron"`            // 告警拉取 cron（与资源同步 cron 独立），空=不自动
	AlertWindowMin int      `gorm:"default:0" json:"alert_window_min"`    // 告警每次拉取的时间窗（分钟），0=默认20；上限15天
	LastAlertAt  *time.Time `json:"last_alert_at"`                        // 上次告警拉取时间
	LastAlertMsg string     `gorm:"size:255" json:"last_alert_msg"`       // 上次告警拉取结果
	Enabled     bool       `gorm:"not null;default:true" json:"enabled"`
	SortOrder   int        `gorm:"not null;default:0" json:"sort_order"` // 自定义排序（小在前，0 视为未排）
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (DataSource) TableName() string { return "data_source" }
