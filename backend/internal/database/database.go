package database

import (
	"fmt"
	"time"

	"github.com/cmdb/cmdb-backend/internal/config"
	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// New 建立 GORM MySQL 连接并配置连接池。
func New(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// Migrate 执行自动建表。各业务模块在此登记模型。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.User{},
		&model.ModelGroup{},
		&model.Model{},
		&model.Attribute{},
		&model.AssociationKind{},
		&model.ModelAssociation{},
		&model.Instance{},
		&model.InstanceRelation{},
		&model.TopoNode{},
		&model.TopoEdge{},
		&model.HostModule{},
		&model.Role{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.AuditLog{},
		&model.DataSource{},
		&model.AlertSource{},
		&model.Alert{},
		&model.Integration{},
		&model.Setting{},
	); err != nil {
		return err
	}
	migrateWebhookDataSources(db)
	return nil
}

// migrateWebhookDataSources 把历史的告警回调类数据源(webhook/nightingale)迁移到独立的 alert_source，
// 保留其 token，然后从 data_source 删除（告警已与数据源解耦）。一次性、幂等。
func migrateWebhookDataSources(db *gorm.DB) {
	var olds []model.DataSource
	if db.Where("type IN ?", []string{"webhook", "nightingale"}).Find(&olds).Error != nil {
		return
	}
	for _, o := range olds {
		var cnt int64
		db.Model(&model.AlertSource{}).Where("name = ? AND type = ?", o.Name, model.AlertSourceWebhook).Count(&cnt)
		if cnt == 0 {
			db.Create(&model.AlertSource{
				Name:    o.Name,
				Type:    model.AlertSourceWebhook,
				Token:   o.Token,
				Enabled: o.Enabled,
			})
		}
		db.Delete(&model.DataSource{}, o.ID)
	}
}
