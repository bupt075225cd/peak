// Package repository 用户数据访问。
package repository

import (
	"context"

	"gorm.io/gorm"

	"peak/libs/domain"
)

// UserRepository 用户数据访问接口。
type UserRepository interface {
	Get(ctx context.Context, id uint64) (*domain.User, error)
	GetByPhone(ctx context.Context, phone string) (*domain.User, error)
	Create(ctx context.Context, u *domain.User) error
	// UpdateName 更新用户昵称，返回更新后的用户。
	UpdateName(ctx context.Context, id uint64, name string) (*domain.User, error)
}

type gormUserRepo struct {
	db *gorm.DB
}

// NewUserRepository 创建 GORM 用户仓储。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &gormUserRepo{db: db}
}

func (r *gormUserRepo) Get(ctx context.Context, id uint64) (*domain.User, error) {
	var u domain.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormUserRepo) GetByPhone(ctx context.Context, phone string) (*domain.User, error) {
	var u domain.User
	if err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormUserRepo) Create(ctx context.Context, u *domain.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *gormUserRepo) UpdateName(ctx context.Context, id uint64, name string) (*domain.User, error) {
	var u domain.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	u.Name = name
	if err := r.db.WithContext(ctx).Save(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
