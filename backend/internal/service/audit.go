package service

import (
	"time"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// AuditService 审计日志查询。
type AuditService struct {
	db *gorm.DB
}

func NewAuditService(db *gorm.DB) *AuditService {
	return &AuditService{db: db}
}

// AuditQuery 审计查询条件。
type AuditQuery struct {
	Username     string
	ResourceType string
	Object       string
	Action       string
	Page         int
	PageSize     int
}

// AuditListResult 审计分页结果。
type AuditListResult struct {
	List     []model.AuditLog `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

func (s *AuditService) List(q AuditQuery) (*AuditListResult, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 500 {
		q.PageSize = 20
	}
	query := s.db.Model(&model.AuditLog{})
	if q.Username != "" {
		query = query.Where("username = ?", q.Username)
	}
	if q.ResourceType != "" {
		query = query.Where("resource_type = ?", q.ResourceType)
	}
	if q.Object != "" {
		query = query.Where("object LIKE ?", "%"+q.Object+"%")
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AuditLog
	if err := query.Order("id desc").
		Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return &AuditListResult{List: list, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// Cleanup 清理审计日志：days<=0 表示删除全部，否则删除 days 天前（created_at 早于 now-days）的记录。
// 返回删除条数。
func (s *AuditService) Cleanup(days int) (int64, error) {
	q := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Model(&model.AuditLog{})
	if days > 0 {
		cutoff := time.Now().AddDate(0, 0, -days)
		q = q.Where("created_at < ?", cutoff)
	}
	r := q.Delete(&model.AuditLog{})
	return r.RowsAffected, r.Error
}
