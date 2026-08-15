package repository

import (
	"context"

	"gorm.io/gorm"

	"peak/libs/domain"
)

// GormRepositories 聚合所有 GORM 仓储实现。
type GormRepositories struct {
	Question QuestionRepository
	Mistake  MistakeRepository
	Category CategoryRepository
	Image    ImageRepository
}

// NewGormRepositories 基于 *gorm.DB 构建仓储实现。
func NewGormRepositories(db *gorm.DB) *GormRepositories {
	return &GormRepositories{
		Question: &gormQuestionRepo{db: db},
		Mistake:  &gormMistakeRepo{db: db},
		Category: &gormCategoryRepo{db: db},
		Image:    &gormImageRepo{db: db},
	}
}

// ---- Question ----

type gormQuestionRepo struct{ db *gorm.DB }

func (r *gormQuestionRepo) Create(ctx context.Context, q *domain.Question) error {
	return r.db.WithContext(ctx).Create(q).Error
}

func (r *gormQuestionRepo) Get(ctx context.Context, id uint64) (*domain.Question, error) {
	var q domain.Question
	err := r.db.WithContext(ctx).Preload("Categories").First(&q, id).Error
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *gormQuestionRepo) List(ctx context.Context, offset, limit int) ([]domain.Question, int64, error) {
	var list []domain.Question
	var total int64
	if err := r.db.WithContext(ctx).Model(&domain.Question{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.db.WithContext(ctx).Preload("Categories").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (r *gormQuestionRepo) Update(ctx context.Context, q *domain.Question) error {
	return r.db.WithContext(ctx).Save(q).Error
}

func (r *gormQuestionRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&domain.Question{}, id).Error
}

// ---- Mistake ----

type gormMistakeRepo struct{ db *gorm.DB }

func (r *gormMistakeRepo) Create(ctx context.Context, m *domain.Mistake) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *gormMistakeRepo) Get(ctx context.Context, id uint64) (*domain.Mistake, error) {
	var m domain.Mistake
	err := r.db.WithContext(ctx).Preload("Question").Preload("Question.Categories").Preload("Images").First(&m, id).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *gormMistakeRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]domain.Mistake, int64, error) {
	var list []domain.Mistake
	var total int64
	q := r.db.WithContext(ctx).Model(&domain.Mistake{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Preload("Question").Preload("Images").Offset(offset).Limit(limit).Find(&list).Error
	return list, total, err
}

func (r *gormMistakeRepo) Update(ctx context.Context, m *domain.Mistake) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *gormMistakeRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&domain.Mistake{}, id).Error
}

// ---- Category ----

type gormCategoryRepo struct{ db *gorm.DB }

func (r *gormCategoryRepo) Create(ctx context.Context, c *domain.Category) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormCategoryRepo) Get(ctx context.Context, id uint64) (*domain.Category, error) {
	var c domain.Category
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *gormCategoryRepo) List(ctx context.Context, typ string) ([]domain.Category, error) {
	var list []domain.Category
	q := r.db.WithContext(ctx)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("sort_order asc").Find(&list).Error
	return list, err
}

func (r *gormCategoryRepo) Update(ctx context.Context, c *domain.Category) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *gormCategoryRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&domain.Category{}, id).Error
}

// ---- Image ----

type gormImageRepo struct{ db *gorm.DB }

func (r *gormImageRepo) Create(ctx context.Context, img *domain.Image) error {
	return r.db.WithContext(ctx).Create(img).Error
}

func (r *gormImageRepo) Get(ctx context.Context, id uint64) (*domain.Image, error) {
	var img domain.Image
	if err := r.db.WithContext(ctx).First(&img, id).Error; err != nil {
		return nil, err
	}
	return &img, nil
}

func (r *gormImageRepo) ListByMistake(ctx context.Context, mistakeID uint64) ([]domain.Image, error) {
	var list []domain.Image
	err := r.db.WithContext(ctx).Where("mistake_id = ?", mistakeID).Find(&list).Error
	return list, err
}
