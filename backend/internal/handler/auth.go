package handler

import (
	"github.com/cmdb/cmdb-backend/internal/auth"
	"github.com/cmdb/cmdb-backend/internal/middleware"
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// AuthHandler 认证相关接口。
type AuthHandler struct {
	users *service.UserService
	token *auth.TokenManager
}

func NewAuthHandler(users *service.UserService, token *auth.TokenManager) *AuthHandler {
	return &AuthHandler{users: users, token: token}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 用户名密码登录，成功返回 JWT 与用户信息。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "用户名和密码不能为空")
		return
	}

	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		switch err {
		case service.ErrInvalidCredential, service.ErrUserDisabled:
			response.Unauthorized(c, err.Error())
		default:
			response.InternalError(c, "登录失败")
		}
		return
	}

	token, err := h.token.Generate(user.ID, user.Username)
	if err != nil {
		response.InternalError(c, "生成令牌失败")
		return
	}

	response.OK(c, gin.H{"token": token, "user": user})
}

// Me 返回当前登录用户信息（需携带有效令牌）。
func (h *AuthHandler) Me(c *gin.Context) {
	uid := c.GetUint(middleware.CtxUserID)
	user, err := h.users.GetByID(uid)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}
	response.OK(c, user)
}
