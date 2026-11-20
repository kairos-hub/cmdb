package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// AlertService 告警源配置与告警接入（拉取 + 回调），与 DataSourceService（资源同步）解耦。
// 告警同步/回调/列表/清理等核心逻辑在 alert.go、alert_webhook.go（接收者同为 *AlertService）。
type AlertService struct {
	db   *gorm.DB
	inst *InstanceService
}

func NewAlertService(db *gorm.DB, inst *InstanceService) *AlertService {
	return &AlertService{db: db, inst: inst}
}

func (s *AlertService) List() ([]model.AlertSource, error) {
	var list []model.AlertSource
	err := s.db.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

// ListOptions 返回告警源精简列表（id/name/type，不含凭证/token），登录用户均可用（供告警事件页筛选）。
func (s *AlertService) ListOptions() ([]DataSourceOption, error) {
	var out []DataSourceOption
	err := s.db.Model(&model.AlertSource{}).Select("id, name, type").Order("sort_order asc, id asc").Scan(&out).Error
	return out, err
}

// ProductDist 按告警关联的 CMDB 资源模型(res_model)分组统计条数，供总览「告警资源分布」扇形图。
// 优先用关联到的 CMDB 模型（服务器/Kafka/MySQL/K8s…）；未关联时退回告警产品；都没有归「未关联」。
func (s *AlertService) ProductDist() ([]NameCount, error) {
	var out []NameCount
	err := s.db.Model(&model.Alert{}).
		Select("COALESCE(NULLIF(res_model, ''), NULLIF(product, ''), '未关联') as name, count(*) as count").
		Group("name").Order("count desc").Scan(&out).Error
	return out, err
}

// Reorder 按给定 id 顺序重排告警源。
func (s *AlertService) Reorder(orderedIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range orderedIDs {
			if err := tx.Model(&model.AlertSource{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *AlertService) Create(a *model.AlertSource) error {
	if a.Type == "" {
		a.Type = model.AlertSourceWebhook
	}
	if strings.TrimSpace(a.Name) == "" {
		return newValidationErr("名称必填")
	}
	switch a.Type {
	case model.AlertSourceAliyun:
		if a.Username == "" || a.Password == "" {
			return newValidationErr("阿里云需填 AccessKeyId 与 AccessKeySecret")
		}
	case model.AlertSourceArms:
		if a.Username == "" || a.Password == "" {
			return newValidationErr("阿里云 ARMS 需填 AccessKeyId 与 AccessKeySecret")
		}
	case model.AlertSourceTencent:
		if a.Username == "" || a.Password == "" {
			return newValidationErr("腾讯云需填 SecretId 与 SecretKey")
		}
	case model.AlertSourcePrometheus:
		if strings.TrimSpace(a.URL) == "" {
			return newValidationErr("Prometheus 告警源需填 Prometheus 地址")
		}
	case model.AlertSourceWebhook:
		if strings.TrimSpace(a.Token) == "" {
			a.Token = genWebhookToken() // 回调校验 token 自动生成
		}
	default:
		return newValidationErr("不支持的告警源类型")
	}
	return s.db.Create(a).Error
}

func (s *AlertService) Update(id uint, a *model.AlertSource) error {
	var existing model.AlertSource
	if err := s.db.First(&existing, id).Error; err != nil {
		return ErrNotFound
	}
	existing.Name = a.Name
	existing.Region = a.Region
	existing.URL = a.URL
	existing.AlertCron = a.AlertCron
	existing.AlertWindowMin = a.AlertWindowMin
	existing.Enabled = a.Enabled
	// 凭证留空表示不改（避免前端回显空密码覆盖）
	if a.Username != "" {
		existing.Username = a.Username
	}
	if a.Password != "" {
		existing.Password = a.Password
	}
	return s.db.Save(&existing).Error
}

func (s *AlertService) Delete(id uint) error {
	return s.db.Delete(&model.AlertSource{}, id).Error
}

// TestConnection 校验云告警源凭证连通性（拉 1 条报警历史，不落库）。
func (s *AlertService) TestConnection(a *model.AlertSource) error {
	now := time.Now()
	start := now.Add(-10 * time.Minute)
	switch a.Type {
	case model.AlertSourceAliyun:
		region := firstAlertRegion(a.Region)
		raw, err := aliRPC("https://metrics."+region+".aliyuncs.com", a.Username, a.Password,
			"2019-01-01", "DescribeAlertLogList", map[string]string{
				"StartTime": strconv.FormatInt(start.UnixMilli(), 10),
				"EndTime":   strconv.FormatInt(now.UnixMilli(), 10),
				"PageNumber": "1", "PageSize": "1",
			})
		if err != nil {
			return err
		}
		var top map[string]interface{}
		_ = json.Unmarshal(raw, &top)
		if code := optStr(top["Code"]); code != "" && code != "200" {
			return newValidationErr("阿里云接口错误：%s", optStr(top["Message"]))
		}
		return nil
	case model.AlertSourceArms:
		region := firstAlertRegion(a.Region)
		if _, err := aliRPC("https://arms."+region+".aliyuncs.com", a.Username, a.Password,
			"2019-08-08", "ListAlerts", map[string]string{"RegionId": region, "Page": "1", "Size": "1"}); err != nil {
			return err
		}
		return nil
	case model.AlertSourceTencent:
		region := firstTencentAlertRegion(a.Region)
		raw, err := tcAPI("monitor", "DescribeAlarmHistories", "2018-07-24", region, a.Username, a.Password,
			map[string]interface{}{"Module": "monitor", "PageNumber": 1, "PageSize": 1, "StartTime": start.Unix(), "EndTime": now.Unix()})
		if err != nil {
			return err
		}
		var wrap map[string]interface{}
		_ = json.Unmarshal(raw, &wrap)
		resp, _ := wrap["Response"].(map[string]interface{})
		if resp == nil {
			return newValidationErr("腾讯云返回异常")
		}
		if e, ok := resp["Error"].(map[string]interface{}); ok {
			return newValidationErr("腾讯云接口错误：%s", optStr(e["Message"]))
		}
		return nil
	case model.AlertSourcePrometheus:
		if _, err := promAlertsGet(a); err != nil {
			return err
		}
		return nil
	default:
		return newValidationErr("回调类型无需测试连接")
	}
}

// StartScheduler 启动告警拉取调度：每分钟检查启用且配了 alert_cron 的云告警源。
func (s *AlertService) StartScheduler() {
	go func() {
		lastRun := map[uint]string{}
		for {
			now := time.Now()
			key := now.Format("2006-01-02 15:04")
			var list []model.AlertSource
			s.db.Where("enabled = ? AND alert_cron <> ''", true).Find(&list)
			for _, a := range list {
				if !validCron(a.AlertCron) || !cronMatch(a.AlertCron, now) {
					continue
				}
				if lastRun[a.ID] == key {
					continue
				}
				lastRun[a.ID] = key
				id := a.ID
				go func() { _, _ = s.SyncAlerts(id) }()
			}
			time.Sleep(time.Until(now.Truncate(time.Minute).Add(time.Minute)))
		}
	}()
}
