// Package service 编排业务逻辑，依赖 repository 接口，便于 mock 测试。
package service

import (
	"context"
	"strings"
	"time"

	"peak/libs/domain"
	"peak/libs/errors"

	"peak/apps/question-service/internal/export"
	"peak/apps/question-service/internal/repository"
)

// Service 业务服务聚合。
type Service struct {
	repos    *repository.GormRepositories
	exporter export.Service
}

// New 创建业务服务实例。exporter 为 nil 时导出能力不可用。
func New(repos *repository.GormRepositories, exporter export.Service) *Service {
	return &Service{repos: repos, exporter: exporter}
}

// CreateQuestion 创建题目（含分类关联）。
func (s *Service) CreateQuestion(ctx context.Context, q *domain.Question) error {
	return s.repos.Question.Create(ctx, q)
}

// GetQuestion 获取题目详情。
func (s *Service) GetQuestion(ctx context.Context, id uint64) (*domain.Question, error) {
	q, err := s.repos.Question.Get(ctx, id)
	if err != nil {
		return nil, errors.Wrap(errors.CodeNotFound, "question not found", err)
	}
	return q, nil
}

// ListQuestions 分页查询题目。
func (s *Service) ListQuestions(ctx context.Context, offset, limit int) ([]domain.Question, int64, error) {
	return s.repos.Question.List(ctx, offset, limit)
}

// UpdateQuestion 更新题目（手动修正入口）。
func (s *Service) UpdateQuestion(ctx context.Context, q *domain.Question) error {
	if _, err := s.repos.Question.Get(ctx, q.ID); err != nil {
		return errors.Wrap(errors.CodeNotFound, "question not found", err)
	}
	return s.repos.Question.Update(ctx, q)
}

// DeleteQuestion 删除题目。
func (s *Service) DeleteQuestion(ctx context.Context, id uint64) error {
	return s.repos.Question.Delete(ctx, id)
}

// CreateMistake 创建错题记录。
func (s *Service) CreateMistake(ctx context.Context, m *domain.Mistake) error {
	if m.UserID == 0 {
		return errors.New(errors.CodeInvalidArgument, "user_id is required")
	}
	if m.QuestionID == 0 {
		return errors.New(errors.CodeInvalidArgument, "question_id is required")
	}
	// 来源必填：录入时要求填写错题出处，便于后续按来源筛选与统计。
	m.Source = strings.TrimSpace(m.Source)
	if m.Source == "" {
		return errors.New(errors.CodeInvalidArgument, "source is required")
	}
	// 前端未传 recorded_at 时由服务端兜底为当前时间，避免入库为零值（0001-01-01）。
	if m.RecordedAt.IsZero() {
		m.RecordedAt = time.Now()
	}
	return s.repos.Mistake.Create(ctx, m)
}

// GetMistake 获取错题详情。
func (s *Service) GetMistake(ctx context.Context, id uint64) (*domain.Mistake, error) {
	m, err := s.repos.Mistake.Get(ctx, id)
	if err != nil {
		return nil, errors.Wrap(errors.CodeNotFound, "mistake not found", err)
	}
	return m, nil
}

// ListMistakes 分页查询用户错题。
func (s *Service) ListMistakes(ctx context.Context, userID uint64, offset, limit int) ([]domain.Mistake, int64, error) {
	return s.repos.Mistake.ListByUser(ctx, userID, offset, limit)
}

// UpdateMistake 更新错题（修正错误原因、掌握程度等）。
func (s *Service) UpdateMistake(ctx context.Context, m *domain.Mistake) error {
	if _, err := s.repos.Mistake.Get(ctx, m.ID); err != nil {
		return errors.Wrap(errors.CodeNotFound, "mistake not found", err)
	}
	return s.repos.Mistake.Update(ctx, m)
}

// DeleteMistake 删除错题。
func (s *Service) DeleteMistake(ctx context.Context, id uint64) error {
	return s.repos.Mistake.Delete(ctx, id)
}

// ListCategories 查询分类列表。
func (s *Service) ListCategories(ctx context.Context, typ string) ([]domain.Category, error) {
	return s.repos.Category.List(ctx, typ)
}

// CreateCategory 创建分类。
func (s *Service) CreateCategory(ctx context.Context, c *domain.Category) error {
	return s.repos.Category.Create(ctx, c)
}
