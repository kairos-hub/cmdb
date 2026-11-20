package service

import (
	"errors"

	"github.com/cmdb/cmdb-backend/internal/model"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredential = errors.New("用户名或密码错误")
	ErrUserDisabled      = errors.New("用户已被禁用")
)

// UserService 用户相关业务逻辑。
type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

// Authenticate 校验用户名密码，返回用户。
func (s *UserService) Authenticate(username, password string) (*model.User, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if !user.CheckPassword(password) {
		return nil, ErrInvalidCredential
	}
	if !user.Enabled() {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

// GetByID 按 ID 查询用户。
func (s *UserService) GetByID(id uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// EnsureAdmin 若系统尚无任何用户，则创建默认超级管理员。返回是否执行了创建。
func (s *UserService) EnsureAdmin(username, password string) (bool, error) {
	var count int64
	if err := s.db.Model(&model.User{}).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	admin := &model.User{
		Username:    username,
		DisplayName: "超级管理员",
		Status:      model.UserStatusEnabled,
	}
	if err := admin.SetPassword(password); err != nil {
		return false, err
	}
	if err := s.db.Create(admin).Error; err != nil {
		return false, err
	}
	return true, nil
}

// List 用户列表。
func (s *UserService) List() ([]model.User, error) {
	var users []model.User
	err := s.db.Order("id asc").Find(&users).Error
	return users, err
}

// Create 创建用户。
func (s *UserService) Create(username, password, displayName, email string) (*model.User, error) {
	var cnt int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&cnt)
	if cnt > 0 {
		return nil, ErrDuplicate
	}
	u := &model.User{Username: username, DisplayName: displayName, Email: email, Status: model.UserStatusEnabled}
	if err := u.SetPassword(password); err != nil {
		return nil, err
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

// Update 更新用户资料与状态。
func (s *UserService) Update(id uint, displayName, email string, status int) (*model.User, error) {
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, ErrNotFound
	}
	u.DisplayName = displayName
	u.Email = email
	u.Status = status
	if err := s.db.Save(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// Delete 删除用户（禁止删除内置 admin）。
func (s *UserService) Delete(id uint) error {
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return ErrNotFound
	}
	if u.Username == "admin" {
		return ErrBuiltinProtected
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Delete(&u).Error
	})
}

// ChangePassword 修改用户密码。
func (s *UserService) ChangePassword(id uint, newPassword string) error {
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return ErrNotFound
	}
	if err := u.SetPassword(newPassword); err != nil {
		return err
	}
	return s.db.Model(&u).Update("password_hash", u.PasswordHash).Error
}
