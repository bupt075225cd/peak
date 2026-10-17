// Package repository 应用注册表数据访问。
package repository

import (
	"context"

	"gorm.io/gorm"

	"peak/libs/domain"
)

// AppClientRepository 应用（前端客户端）注册表数据访问接口。
type AppClientRepository interface {
	// GetByClientID 按应用标识查询注册记录。
	GetByClientID(ctx context.Context, clientID string) (*domain.AppClient, error)
}

type gormAppClientRepo struct {
	db *gorm.DB
}

// NewAppClientRepository 创建 GORM 应用注册表仓储。
func NewAppClientRepository(db *gorm.DB) AppClientRepository {
	return &gormAppClientRepo{db: db}
}

func (r *gormAppClientRepo) GetByClientID(ctx context.Context, clientID string) (*domain.AppClient, error) {
	var c domain.AppClient
	if err := r.db.WithContext(ctx).Where("client_id = ?", clientID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
