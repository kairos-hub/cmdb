package handler

import (
	"strconv"

	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// AuditHandler 审计日志查询。
type AuditHandler struct {
	svc *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

func (h *AuditHandler) List(c *gin.Context) {
	page, pageSize := parsePaging(c)
	q := service.AuditQuery{
		Username:     c.Query("username"),
		ResourceType: c.Query("resource_type"),
		Object:       c.Query("object"),
		Action:       c.Query("action"),
		Page:         page,
		PageSize:     pageSize,
	}
	res, err := h.svc.List(q)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, res)
}

// Cleanup 清理审计日志：query days<=0 删除全部，否则删除 days 天前的记录。
func (h *AuditHandler) Cleanup(c *gin.Context) {
	days, _ := strconv.Atoi(c.Query("days"))
	if days < 0 {
		days = 0
	}
	n, err := h.svc.Cleanup(days)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": n})
}
