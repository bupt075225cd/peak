package repository

import (
	"context"
	"strings"

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

// mistakeSourceSQL 错题来源的 SQL 表达式：优先题目来源，为空再取错题记录来源。
// 与 service/export.go 的 mistakeSource 取值口径保持一致。
const mistakeSourceSQL = "COALESCE(NULLIF(TRIM(questions.source), ''), TRIM(mistakes.source))"

// mistakeBaseQuery 错题列表的基础查询：限定用户，并 JOIN 未删除的题目表以支持按题目字段过滤。
func (r *gormMistakeRepo) mistakeBaseQuery(ctx context.Context, userID uint64) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&domain.Mistake{}).
		Joins("JOIN questions ON questions.id = mistakes.question_id AND questions.deleted_at IS NULL").
		Where("mistakes.user_id = ?", userID)
}

// mistakeKeywordFilter 生成关键词的条件与参数。
//
// 关键词按空白拆成多个词，每个词都必须命中 题干/知识点/题型/来源/学科 之一
// （词之间 AND、字段之间 OR）；字段统一用 LOWER 比较，
// 抹平 MySQL 与 SQLite 在 LIKE 大小写敏感性上的差异。
func mistakeKeywordFilter(keyword string) (string, []any) {
	terms := strings.Fields(keyword)
	if len(terms) == 0 {
		return "", nil
	}

	clauses := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)*5)
	for _, term := range terms {
		pattern := "%" + strings.ToLower(term) + "%"
		clauses = append(clauses, "(LOWER(questions.stem_text) LIKE ?"+
			" OR LOWER(questions.knowledge_points) LIKE ?"+
			" OR LOWER(questions.question_type) LIKE ?"+
			" OR LOWER(questions.subject) LIKE ?"+
			" OR LOWER("+mistakeSourceSQL+") LIKE ?)")
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	return strings.Join(clauses, " AND "), args
}

// applyMistakeFilter 追加关键词、学科与来源过滤条件。
func applyMistakeFilter(q *gorm.DB, query domain.MistakeQuery) *gorm.DB {
	if cond, args := mistakeKeywordFilter(query.Keyword); cond != "" {
		q = q.Where(cond, args...)
	}
	if s := strings.TrimSpace(query.Subject); s != "" {
		q = q.Where("questions.subject = ?", s)
	}
	if s := strings.TrimSpace(query.Source); s != "" {
		q = q.Where(mistakeSourceSQL+" = ?", s)
	}
	return q
}

// mistakeFacetRow 分面计数的一行。
type mistakeFacetRow struct {
	Name  string
	Count int64
}

// ListByUser 按条件分页查询用户错题，并返回总数与学科/来源分面计数。
func (r *gormMistakeRepo) ListByUser(ctx context.Context, query domain.MistakeQuery) (domain.MistakeListResult, error) {
	result := domain.MistakeListResult{
		SubjectCounts: map[string]int64{},
		SourceCounts:  map[string]int64{},
	}

	// 总数：与列表使用同一套过滤条件。
	if err := applyMistakeFilter(r.mistakeBaseQuery(ctx, query.UserID), query).
		Count(&result.Total).Error; err != nil {
		return domain.MistakeListResult{}, err
	}

	// 当前页数据。JOIN 后必须显式 Select("mistakes.*")，否则两表同名列会冲突；
	// 排序必须显式且稳定，否则 offset/limit 分页可能重复或漏项。
	if err := applyMistakeFilter(r.mistakeBaseQuery(ctx, query.UserID), query).
		Select("mistakes.*").
		Preload("Question").Preload("Images").
		Order("mistakes.recorded_at DESC, mistakes.id DESC").
		Offset(query.Offset).Limit(query.Limit).
		Find(&result.Items).Error; err != nil {
		return domain.MistakeListResult{}, err
	}

	// 分面计数只按关键词统计：若带上学科/来源过滤，切换筛选后其它分支会变成 0。
	subjectCounts, sourceCounts, err := r.mistakeFacets(ctx, query.UserID, query.Keyword)
	if err != nil {
		return domain.MistakeListResult{}, err
	}
	result.SubjectCounts = subjectCounts
	result.SourceCounts = sourceCounts
	return result, nil
}

// mistakeFacets 统计关键词条件下的学科与来源分布（来源为空的历史数据不展示）。
func (r *gormMistakeRepo) mistakeFacets(ctx context.Context, userID uint64, keyword string) (map[string]int64, map[string]int64, error) {
	subjectCounts := make(map[string]int64)
	sourceCounts := make(map[string]int64)

	// 学科分布。
	subjectQuery := r.mistakeBaseQuery(ctx, userID)
	if cond, args := mistakeKeywordFilter(keyword); cond != "" {
		subjectQuery = subjectQuery.Where(cond, args...)
	}
	var rows []mistakeFacetRow
	if err := subjectQuery.
		Select("questions.subject AS name, COUNT(*) AS count").
		Group("questions.subject").
		Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		if row.Name != "" {
			subjectCounts[row.Name] = row.Count
		}
	}

	// 来源分布。
	sourceQuery := r.mistakeBaseQuery(ctx, userID)
	if cond, args := mistakeKeywordFilter(keyword); cond != "" {
		sourceQuery = sourceQuery.Where(cond, args...)
	}
	rows = nil
	if err := sourceQuery.
		Select(mistakeSourceSQL + " AS name, COUNT(*) AS count").
		Group(mistakeSourceSQL).
		Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		if row.Name != "" {
			sourceCounts[row.Name] = row.Count
		}
	}

	return subjectCounts, sourceCounts, nil
}

func (r *gormMistakeRepo) ListByIDs(ctx context.Context, userID uint64, ids []uint64) ([]domain.Mistake, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []domain.Mistake
	// 必须带 user_id 过滤，避免越权导出他人错题。
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND id IN ?", userID, ids).
		Preload("Question").
		Find(&list).Error
	return list, err
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
