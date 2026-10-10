// Package service 编排业务逻辑，依赖 repository 接口，便于 mock 测试。
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"peak/libs/domain"
	bizerrors "peak/libs/errors"
	"peak/libs/observability"

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
		return nil, bizerrors.Wrap(bizerrors.CodeNotFound, "question not found", err)
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
		return bizerrors.Wrap(bizerrors.CodeNotFound, "question not found", err)
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
		return bizerrors.New(bizerrors.CodeInvalidArgument, "user_id is required")
	}
	if m.QuestionID == 0 {
		return bizerrors.New(bizerrors.CodeInvalidArgument, "question_id is required")
	}
	// 来源必填：录入时要求填写错题出处，便于后续按来源筛选与统计。
	m.Source = strings.TrimSpace(m.Source)
	if m.Source == "" {
		return bizerrors.New(bizerrors.CodeInvalidArgument, "source is required")
	}
	// 前端未传 recorded_at 时由服务端兜底为当前时间，避免入库为零值（0001-01-01）。
	if m.RecordedAt.IsZero() {
		m.RecordedAt = time.Now()
	}
	err := s.repos.Mistake.Create(ctx, m)
	observability.ObserveMistakeOp("create", err)
	return err
}

// GetMistake 获取错题详情。
func (s *Service) GetMistake(ctx context.Context, id uint64) (*domain.Mistake, error) {
	m, err := s.repos.Mistake.Get(ctx, id)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeNotFound, "mistake not found", err)
	}
	return m, nil
}

// ListMistakes 按条件分页查询用户错题，并返回学科/来源分面计数。
func (s *Service) ListMistakes(ctx context.Context, query domain.MistakeQuery) (domain.MistakeListResult, error) {
	return s.repos.Mistake.ListByUser(ctx, query)
}

// UpdateMistake 更新错题（修正错误原因、掌握程度等）。
func (s *Service) UpdateMistake(ctx context.Context, m *domain.Mistake) error {
	if _, err := s.repos.Mistake.Get(ctx, m.ID); err != nil {
		return bizerrors.Wrap(bizerrors.CodeNotFound, "mistake not found", err)
	}
	return s.repos.Mistake.Update(ctx, m)
}

// DeleteMistake 彻底删除错题及其全部关联数据（题目、图片记录、分类关联、
// 识别任务），返回需要物理删除的存储文件 key（由 handler 尽力删除）。
func (s *Service) DeleteMistake(ctx context.Context, userID, id uint64) ([]string, error) {
	keys, err := s.repos.Mistake.Purge(ctx, userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, bizerrors.New(bizerrors.CodeNotFound, "mistake not found")
		}
		return nil, err
	}
	observability.ObserveMistakeOp("delete", nil)
	return keys, nil
}

// ListCategories 查询分类列表。
func (s *Service) ListCategories(ctx context.Context, typ string) ([]domain.Category, error) {
	return s.repos.Category.List(ctx, typ)
}

// CreateCategory 创建分类。
func (s *Service) CreateCategory(ctx context.Context, c *domain.Category) error {
	return s.repos.Category.Create(ctx, c)
}
