package handler

import (
	"github.com/cmdb/cmdb-backend/internal/model"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// IntegrationHandler 系统集成入口配置。
type IntegrationHandler struct {
	svc *service.IntegrationService
}

func NewIntegrationHandler(svc *service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

func (h *IntegrationHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, list)
}

func (h *IntegrationHandler) Create(c *gin.Context) {
	var it model.Integration
	if err := c.ShouldBindJSON(&it); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Create(&it); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, it)
}

func (h *IntegrationHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var it model.Integration
	if err := c.ShouldBindJSON(&it); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Update(id, &it); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *IntegrationHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *IntegrationHandler) Reorder(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.svc.Reorder(req.IDs); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}
