package database

import (
	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

// CleanupLegacyBelong 删除旧版「属于业务(belong_*_biz)」的模型关联定义及其实例关联数据。
// 业务归属已统一由业务拓扑（host_module）承载，这些关联不再使用。
// 幂等：无匹配时不做任何改动。
func CleanupLegacyBelong(db *gorm.DB) (relRows int64, assocRows int64, err error) {
	var assocs []model.ModelAssociation
	if err = db.Where("code LIKE ?", "belong_%_biz").Find(&assocs).Error; err != nil {
		return
	}
	if len(assocs) == 0 {
		return
	}
	ids := make([]uint, 0, len(assocs))
	for _, a := range assocs {
		ids = append(ids, a.ID)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		r := tx.Where("association_id IN ?", ids).Delete(&model.InstanceRelation{})
		if r.Error != nil {
			return r.Error
		}
		relRows = r.RowsAffected
		a := tx.Where("id IN ?", ids).Delete(&model.ModelAssociation{})
		if a.Error != nil {
			return a.Error
		}
		assocRows = a.RowsAffected
		return nil
	})
	return
}
