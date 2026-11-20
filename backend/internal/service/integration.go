package service

import (
	"strings"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// IntegrationService 系统集成（外部 Web 服务入口）配置。
type IntegrationService struct {
	db *gorm.DB
}

func NewIntegrationService(db *gorm.DB) *IntegrationService {
	return &IntegrationService{db: db}
}

func (s *IntegrationService) List() ([]model.Integration, error) {
	var list []model.Integration
	err := s.db.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

func (s *IntegrationService) Create(it *model.Integration) error {
	if strings.TrimSpace(it.Name) == "" {
		return newValidationErr("名称必填")
	}
	if strings.TrimSpace(it.URL) == "" {
		return newValidationErr("地址必填")
	}
	if it.OpenMode != model.IntegrationBlank {
		it.OpenMode = model.IntegrationEmbed
	}
	return s.db.Create(it).Error
}

func (s *IntegrationService) Update(id uint, it *model.Integration) error {
	var existing model.Integration
	if err := s.db.First(&existing, id).Error; err != nil {
		return ErrNotFound
	}
	existing.Name = it.Name
	existing.URL = it.URL
	existing.Icon = it.Icon
	existing.Description = it.Description
	existing.Enabled = it.Enabled
	if it.OpenMode == model.IntegrationBlank {
		existing.OpenMode = model.IntegrationBlank
	} else {
		existing.OpenMode = model.IntegrationEmbed
	}
	return s.db.Save(&existing).Error
}

func (s *IntegrationService) Delete(id uint) error {
	return s.db.Delete(&model.Integration{}, id).Error
}

// Reorder 按给定 id 顺序重排。
func (s *IntegrationService) Reorder(orderedIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range orderedIDs {
			if err := tx.Model(&model.Integration{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
