package handler

import (
	"net/http"

	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthHandler 健康检查，含数据库连通性探测。
type HealthHandler struct {
	db *gorm.DB
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Check(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		response.Fail(c, http.StatusServiceUnavailable, response.CodeInternalError, "数据库不可用")
		return
	}
	response.OK(c, gin.H{"status": "ok", "db": "ok"})
}
