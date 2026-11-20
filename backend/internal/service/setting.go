package service

import (
	"strconv"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingService 全局键值配置。
type SettingService struct {
	db *gorm.DB
}

func NewSettingService(db *gorm.DB) *SettingService {
	return &SettingService{db: db}
}

// All 返回全部设置（map）。
func (s *SettingService) All() map[string]string {
	var list []model.Setting
	s.db.Find(&list)
	out := map[string]string{}
	for _, it := range list {
		out[it.Key] = it.Value
	}
	return out
}

// Get 取设置，缺省返回 def。
func (s *SettingService) Get(key, def string) string {
	var it model.Setting
	if s.db.First(&it, "`key` = ?", key).Error != nil {
		return def
	}
	return it.Value
}

// GetInt 取整型设置。
func (s *SettingService) GetInt(key string, def int) int {
	if n, err := strconv.Atoi(s.Get(key, "")); err == nil {
		return n
	}
	return def
}

// SetMany 批量 upsert 设置。
func (s *SettingService) SetMany(kv map[string]string) error {
	for k, v := range kv {
		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&model.Setting{Key: k, Value: v}).Error; err != nil {
			return err
		}
	}
	return nil
}

// StartOfflineCleanup 启动「已下线」实例清理调度：按设置的 cron 执行，删除超期下线实例。
func StartOfflineCleanup(setting *SettingService, inst *InstanceService) {
	go func() {
		last := ""
		for {
			now := time.Now()
			key := now.Format("2006-01-02 15:04")
			if setting.Get(model.SettingOfflineCleanupEnabled, "0") == "1" {
				cron := setting.Get(model.SettingOfflineCleanupCron, "0 3 * * *")
				if validCron(cron) && cronMatch(cron, now) && last != key {
					last = key
					_, _ = inst.CleanupOfflineInstances(setting.GetInt(model.SettingOfflineCleanupDays, 7))
				}
			}
			time.Sleep(time.Until(now.Truncate(time.Minute).Add(time.Minute)))
		}
	}()
}

// StartAlertCleanup 启动告警清理调度：按设置的 cron 执行，删除超过保留天数的告警。
func StartAlertCleanup(setting *SettingService, ds *AlertService) {
	go func() {
		last := ""
		for {
			now := time.Now()
			key := now.Format("2006-01-02 15:04")
			if setting.Get(model.SettingAlertCleanupEnabled, "1") == "1" {
				cron := setting.Get(model.SettingAlertCleanupCron, "0 3 * * *")
				if validCron(cron) && cronMatch(cron, now) && last != key {
					last = key
					days := setting.GetInt(model.SettingAlertCleanupDays, 90)
					if days > 0 {
						_, _ = ds.CleanupAlerts(days, 0) // 定时清理覆盖全部数据源
					}
				}
			}
			time.Sleep(time.Until(now.Truncate(time.Minute).Add(time.Minute)))
		}
	}()
}

// StartAuditCleanup 启动审计日志清理调度：按设置的 cron 执行，删除超过保留天数的审计日志。
func StartAuditCleanup(setting *SettingService, audit *AuditService) {
	go func() {
		last := ""
		for {
			now := time.Now()
			key := now.Format("2006-01-02 15:04")
			if setting.Get(model.SettingAuditCleanupEnabled, "0") == "1" {
				cron := setting.Get(model.SettingAuditCleanupCron, "0 4 * * *")
				if validCron(cron) && cronMatch(cron, now) && last != key {
					last = key
					days := setting.GetInt(model.SettingAuditCleanupDays, 90)
					if days > 0 {
						_, _ = audit.Cleanup(days)
					}
				}
			}
			time.Sleep(time.Until(now.Truncate(time.Minute).Add(time.Minute)))
		}
	}()
}
