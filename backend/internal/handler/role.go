package handler

import (
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// RoleHandler 角色与权限管理。
type RoleHandler struct {
	rbac *service.RBACService
}

func NewRoleHandler(rbac *service.RBACService) *RoleHandler {
	return &RoleHandler{rbac: rbac}
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.rbac.ListRoles()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, roles)
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req struct {
		Code        string   `json:"code" binding:"required"`
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Functions   []string `json:"functions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "标识和名称必填")
		return
	}
	r, err := h.rbac.CreateRole(req.Code, req.Name, req.Description, req.Functions)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, r)
}

func (h *RoleHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Description string   `json:"description"`
		Functions   []string `json:"functions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	r, err := h.rbac.UpdateRole(id, req.Name, req.Description, req.Functions)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, r)
}

func (h *RoleHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.rbac.DeleteRole(id); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *RoleHandler) GetPermissions(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	perms, err := h.rbac.GetRolePermissions(id)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, perms)
}

func (h *RoleHandler) SetPermissions(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Permissions []service.PermInput `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.rbac.SetRolePermissions(id, req.Permissions); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}
