package handler

import (
	"errors"

	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// mapServiceError 将 service 哨兵错误映射为 HTTP 状态码（用户/角色/审计 handler 共用）。
func mapServiceError(c *gin.Context, err error) {
	var ve *service.ValidationError
	switch {
	case errors.As(err, &ve):
		response.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrDuplicate):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrBuiltinProtected):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrInvalidRef):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, err.Error())
	}
}

// UserHandler 用户管理与当前用户权限。
type UserHandler struct {
	users *service.UserService
	rbac  *service.RBACService
}

func NewUserHandler(users *service.UserService, rbac *service.RBACService) *UserHandler {
	return &UserHandler{users: users, rbac: rbac}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, users)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "用户名和密码必填")
		return
	}
	u, err := h.users.Create(req.Username, req.Password, req.DisplayName, req.Email)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, u)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Status      int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	u, err := h.users.Update(id, req.DisplayName, req.Email, req.Status)
	if err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, u)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.users.Delete(id); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "密码不能为空")
		return
	}
	if err := h.users.ChangePassword(id, req.Password); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *UserHandler) GetRoles(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	response.OK(c, h.rbac.GetUserRoleIDs(id))
}

func (h *UserHandler) SetRoles(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		RoleIDs []uint `json:"role_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if err := h.rbac.SetUserRoles(id, req.RoleIDs); err != nil {
		mapServiceError(c, err)
		return
	}
	response.OK(c, nil)
}

// CurrentPermissions 返回当前登录用户的有效权限（供前端控制）。
func (h *UserHandler) CurrentPermissions(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	response.OK(c, h.rbac.GetUserPermissions(uid))
}
