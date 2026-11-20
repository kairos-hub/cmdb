package middleware

import (
	"github.com/cmdb/cmdb-backend/internal/service"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// AdminRequired 要求当前用户拥有超级管理员角色。
func AdminRequired(rbac *service.RBACService) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetUint(CtxUserID)
		if !rbac.IsAdmin(uid) {
			response.Forbidden(c, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// FuncRequired 要求当前用户拥有指定功能权限（超管恒通过）。
func FuncRequired(rbac *service.RBACService, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetUint(CtxUserID)
		if !rbac.HasFunction(uid, code) {
			response.Forbidden(c, "无该功能的操作权限")
			c.Abort()
			return
		}
		c.Next()
	}
}
