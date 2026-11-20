package handler

import (
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// SettingHandler 全局设置。
type SettingHandler struct {
	svc *service.SettingService
}

func NewSettingHandler(svc *service.SettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

func (h *SettingHandler) Get(c *gin.Context) {
	response.OK(c, h.svc.All())
}

func (h *SettingHandler) Update(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.SetMany(req); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
