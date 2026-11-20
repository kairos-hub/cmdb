package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 用户状态
const (
	UserStatusDisabled = 0
	UserStatusEnabled  = 1
)

// User 系统用户。
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	DisplayName  string    `gorm:"size:64" json:"display_name"`
	Email        string    `gorm:"size:128" json:"email"`
	Status       int       `gorm:"default:1;not null" json:"status"` // 1 启用 / 0 禁用
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "user" }

// SetPassword 用 bcrypt 哈希并写入密码。
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword 校验明文密码是否匹配。
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

// Enabled 返回用户是否处于启用状态。
func (u *User) Enabled() bool { return u.Status == UserStatusEnabled }
