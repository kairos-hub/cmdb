package middleware

import (
	"strings"
	"time"

	"github.com/cmdb/cmdb-backend/internal/auth"
	"github.com/cmdb/cmdb-backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// 存入 gin.Context 的键
const (
	CtxUserID   = "userID"
	CtxUsername = "username"
	// 审计覆盖：handler 可写入这些键影响审计中间件
	CtxAuditSkip     = "auditSkip"     // bool：跳过本次审计记录
	CtxAuditAction   = "auditAction"   // string：覆盖动作名
	CtxAuditChanges  = "auditChanges"  // []string：变更明细（更新内容）
	CtxAuditObject   = "auditObject"   // string：覆盖操作对象
	CtxAuditInstance = "auditInstance" // string：覆盖操作目标/实例名
)

// Auth 校验 Bearer JWT，解析后将用户信息写入上下文。
func Auth(tm *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Unauthorized(c, "未提供认证令牌")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := tm.Parse(tokenStr)
		if err != nil {
			response.Unauthorized(c, err.Error())
			c.Abort()
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		// 活跃自动续期：令牌已过半有效期时下发新令牌（请求头 X-Refreshed-Token），前端更新后持续在线
		if claims.ExpiresAt != nil && claims.IssuedAt != nil {
			life := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
			if life > 0 && time.Since(claims.IssuedAt.Time) > life/2 {
				if nt, err := tm.Generate(claims.UserID, claims.Username); err == nil {
					c.Header("X-Refreshed-Token", nt)
				}
			}
		}
		c.Next()
	}
}
