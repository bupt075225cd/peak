// Package repository 定义数据访问接口与 GORM 实现，供 service 层依赖。
package repository

import (
	"context"

	"peak/libs/domain"
)

// QuestionRepository 题目数据访问接口。
type QuestionRepository interface {
	Create(ctx context.Context, q *domain.Question) error
	Get(ctx context.Context, id uint64) (*domain.Question, error)
	List(ctx context.Context, offset, limit int) ([]domain.Question, int64, error)
	Update(ctx context.Context, q *domain.Question) error
	Delete(ctx context.Context, id uint64) error
}

// MistakeRepository 错题数据访问接口。
type MistakeRepository interface {
	Create(ctx context.Context, m *domain.Mistake) error
	Get(ctx context.Context, id uint64) (*domain.Mistake, error)
	ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]domain.Mistake, int64, error)
	Update(ctx context.Context, m *domain.Mistake) error
	Delete(ctx context.Context, id uint64) error
}

// CategoryRepository 分类数据访问接口。
type CategoryRepository interface {
	Create(ctx context.Context, c *domain.Category) error
	Get(ctx context.Context, id uint64) (*domain.Category, error)
	List(ctx context.Context, typ string) ([]domain.Category, error)
	Update(ctx context.Context, c *domain.Category) error
	Delete(ctx context.Context, id uint64) error
}

// ImageRepository 图片数据访问接口。
type ImageRepository interface {
	Create(ctx context.Context, img *domain.Image) error
	Get(ctx context.Context, id uint64) (*domain.Image, error)
	ListByMistake(ctx context.Context, mistakeID uint64) ([]domain.Image, error)
}
