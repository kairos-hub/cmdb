package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken 令牌无效或已过期。
var ErrInvalidToken = errors.New("令牌无效或已过期")

// Claims 自定义 JWT 载荷。
type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// TokenManager 负责 JWT 的签发与解析。
type TokenManager struct {
	secret      []byte
	expireHours int
}

func NewTokenManager(secret string, expireHours int) *TokenManager {
	if expireHours <= 0 {
		expireHours = 24 * 30 // 默认 30 天
	}
	return &TokenManager{secret: []byte(secret), expireHours: expireHours}
}

// Generate 为指定用户签发令牌。
func (m *TokenManager) Generate(userID uint, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(m.expireHours) * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse 校验并解析令牌，返回载荷。
func (m *TokenManager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
