package repository

import (
	"context"
	"encoding/json"
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

// Purge 在单个事务内彻底删除错题及其全部关联数据：
//   - 错题记录、错题关联的图片记录与识别任务（硬删除）；
//   - 题目在无其他错题引用时一并硬删除，并清理题目-分类关联。
//
// 返回需要从存储中物理删除的正式区文件 key（由 handler 尽力删除，
// 文件删除失败不影响数据库清理结果）。错题不存在或不属于该用户返回
// gorm.ErrRecordNotFound，由上层转译为业务 NotFound。
func (r *gormMistakeRepo) Purge(ctx context.Context, userID, id uint64) ([]string, error) {
	var keys []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keys = nil

		// 1. 定位错题：带 user_id 过滤，避免越权删除他人错题。
		var m domain.Mistake
		if err := tx.Where("user_id = ? AND id = ?", userID, id).First(&m).Error; err != nil {
			return err
		}

		// 2. 关联题目与配图引用（提交后均为 committed/ 正式区 key）。
		var q domain.Question
		if err := tx.First(&q, m.QuestionID).Error; err != nil {
			return err
		}
		keys = committedImageKeys(q.Image)

		// 3. 错题关联的图片记录及其识别任务。
		var imgIDs []uint64
		if err := tx.Unscoped().Model(&domain.Image{}).
			Where("mistake_id = ?", m.ID).Pluck("id", &imgIDs).Error; err != nil {
			return err
		}
		if len(imgIDs) > 0 {
			if err := tx.Unscoped().Where("image_id IN ?", imgIDs).
				Delete(&domain.RecognitionTask{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Unscoped().Where("mistake_id = ?", m.ID).
			Delete(&domain.Image{}).Error; err != nil {
			return err
		}

		// 4. 题目是否仍被其他（未删除的）错题引用：决定题目是否级联删除。
		var refs int64
		if err := tx.Model(&domain.Mistake{}).
			Where("question_id = ? AND id <> ?", q.ID, m.ID).
			Count(&refs).Error; err != nil {
			return err
		}

		// 5. 删除错题记录本身。
		if err := tx.Unscoped().Delete(&domain.Mistake{}, m.ID).Error; err != nil {
			return err
		}

		// 6. 无其他引用时硬删除题目，并清理题目-分类多对多关联。
		if refs == 0 {
			if err := tx.Exec("DELETE FROM question_categories WHERE question_id = ?", q.ID).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Delete(&domain.Question{}, q.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// committedImageKeys 从 question.image JSON 引用列表提取需物理删除的正式区 key。
//
// 提交后引用均带 committed/ 前缀；存量无前缀的旧数据按正式区访问路径
// （committed/<key>，与 getMistakeFile 的读取路径一致）补齐；
// transient/ 临时区引用属识别服务存储，不在本服务存储内，跳过。
// 脏数据（JSON 解析失败）不阻断删除流程。
func committedImageKeys(imageJSON string) []string {
	if strings.TrimSpace(imageJSON) == "" {
		return nil
	}
	var refs []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(imageJSON), &refs); err != nil {
		return nil
	}
	keys := make([]string, 0, len(refs))
	for _, r := range refs {
		k := strings.TrimSpace(r.Key)
		if k == "" {
			continue
		}
		switch {
		case strings.HasPrefix(k, domain.CommittedPrefix):
			keys = append(keys, k)
		case strings.HasPrefix(k, domain.TransientPrefix):
			// 未晋升的临时区产物，不在本服务存储内。
		default:
			keys = append(keys, domain.CommittedPrefix+k)
		}
	}
	return keys
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
