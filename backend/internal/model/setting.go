package model

import "time"

// Setting 全局键值配置。
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"size:255" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Setting) TableName() string { return "setting" }

// 设置项键
const (
	SettingOfflineCleanupEnabled = "offline_cleanup_enabled" // "1"/"0"
	SettingOfflineCleanupDays    = "offline_cleanup_days"    // 天数
	SettingOfflineCleanupCron    = "offline_cleanup_cron"    // cron 表达式
	SettingAuditCleanupEnabled   = "audit_cleanup_enabled"   // 审计日志定时清理 "1"/"0"
	SettingAuditCleanupDays      = "audit_cleanup_days"      // 保留天数（删除超过该天数的日志）
	SettingAuditCleanupCron      = "audit_cleanup_cron"      // cron 表达式
	SettingAlertCleanupEnabled   = "alert_cleanup_enabled"   // 告警定时清理 "1"/"0"
	SettingAlertCleanupDays      = "alert_cleanup_days"      // 告警保留天数
	SettingAlertCleanupCron      = "alert_cleanup_cron"      // cron 表达式
	SettingDemoSeeded            = "demo_data_seeded"        // "1"：演示数据已播种过，不再重播（尊重用户删除）
	SettingFieldOrderSeeded      = "field_order_seeded"      // "1"：内置字段顺序归一化已跑过，之后不再重刷（尊重用户手动排序）
)
