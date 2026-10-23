// Package repository 古诗文数据访问（GORM）。
package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"peak/libs/domain"
)

// AuthorCount 作者作品数统计行。
type AuthorCount struct {
	Author string `json:"author"`
	Count  int64  `json:"count"`
}

// VolumeCount 册数 × 课内外统计行。
type VolumeCount struct {
	Volume   string `json:"volume"`
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

// PoemRepository 古诗文仓储接口。
type PoemRepository interface {
	List(ctx context.Context, f Filter) ([]domain.Poem, int64, error)
	GetByID(ctx context.Context, id uint64) (*domain.Poem, error)
	Related(ctx context.Context, poem *domain.Poem, limit int) ([]domain.Poem, error)
	CountByDynasty(ctx context.Context) (map[string]int64, error)
	CountByVolume(ctx context.Context) ([]VolumeCount, error)
	AuthorRanking(ctx context.Context, top int) ([]AuthorCount, error)
	AllContents(ctx context.Context) ([]string, error)
}

// Filter 列表筛选条件；空值表示不启用该维度。
type Filter struct {
	Dynasty  string
	Author   string
	Volume   string
	Category string
	Keyword  string
	Offset   int
	Limit    int
}

// gormRepo GORM 实现。
type gormRepo struct {
	db *gorm.DB
}

// NewGormPoemRepository 创建仓储实例。
func NewGormPoemRepository(db *gorm.DB) PoemRepository {
	return &gormRepo{db: db}
}

// List 按筛选条件分页查询篇目；keyword 对篇名/作者/正文做包含匹配。
func (r *gormRepo) List(ctx context.Context, f Filter) ([]domain.Poem, int64, error) {
	query := r.poemModel(ctx, f)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	var poems []domain.Poem
	err := query.
		Order("dynasty_order ASC, id ASC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&poems).Error
	return poems, total, err
}

// poemModel 构建带筛选条件的查询（不含排序/分页）。
func (r *gormRepo) poemModel(ctx context.Context, f Filter) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&domain.Poem{})
	if f.Dynasty != "" {
		query = query.Where("dynasty = ?", f.Dynasty)
	}
	if f.Author != "" {
		query = query.Where("author = ?", f.Author)
	}
	if f.Volume != "" {
		query = query.Where("volume = ?", f.Volume)
	}
	if f.Category != "" {
		query = query.Where("category = ?", f.Category)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		// 统一按小写匹配：PostgreSQL 的 LIKE 区分大小写，SQLite 默认不区分，
		// 用 LOWER 对齐两者行为（与错题关键词检索口径一致）。
		like := "%" + strings.ToLower(kw) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(author) LIKE ? OR LOWER(content) LIKE ?", like, like, like)
	}
	return query
}

// GetByID 按主键查询篇目；不存在返回 nil, nil。
func (r *gormRepo) GetByID(ctx context.Context, id uint64) (*domain.Poem, error) {
	var p domain.Poem
	err := r.db.WithContext(ctx).First(&p, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Related 相关篇目推荐：同作者优先，其余按同朝代，排除自身，按 limit 截断。
// gorm 的 Order 不支持参数绑定，分两步查询：先同作者，再同朝代补足并去重。
func (r *gormRepo) Related(ctx context.Context, poem *domain.Poem, limit int) ([]domain.Poem, error) {
	if limit <= 0 {
		limit = 4
	}
	var out []domain.Poem
	seen := map[uint64]bool{poem.ID: true}

	appendRows := func(query *gorm.DB) error {
		var rows []domain.Poem
		if err := query.Order("dynasty_order ASC, id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, p := range rows {
			if len(out) >= limit {
				return nil
			}
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
		}
		return nil
	}

	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&domain.Poem{}).Where("id <> ?", poem.ID)
	}
	if err := appendRows(base().Where("author = ?", poem.Author)); err != nil {
		return nil, err
	}
	if len(out) < limit {
		if err := appendRows(base().Where("dynasty = ?", poem.Dynasty)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountByDynasty 按朝代统计篇目数。
func (r *gormRepo) CountByDynasty(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Dynasty string `json:"dynasty"`
		Count   int64  `json:"count"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&domain.Poem{}).
		Select("dynasty, COUNT(*) AS count").
		Group("dynasty").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Dynasty] = r.Count
	}
	return out, nil
}

// CountByVolume 按册数 × 课内外统计篇目数。
func (r *gormRepo) CountByVolume(ctx context.Context) ([]VolumeCount, error) {
	var rows []VolumeCount
	err := r.db.WithContext(ctx).Model(&domain.Poem{}).
		Select("volume, category, COUNT(*) AS count").
		Group("volume, category").
		Find(&rows).Error
	return rows, err
}

// AuthorRanking 作者作品数排行（降序，top ≤ 0 时返回全部）。
func (r *gormRepo) AuthorRanking(ctx context.Context, top int) ([]AuthorCount, error) {
	query := r.db.WithContext(ctx).Model(&domain.Poem{}).
		Select("author, COUNT(*) AS count").
		Group("author").
		Order("count DESC, author ASC")
	if top > 0 {
		query = query.Limit(top)
	}
	var rows []AuthorCount
	err := query.Find(&rows).Error
	return rows, err
}

// AllContents 取全部正文（词频统计用，仅返回内容列）。
func (r *gormRepo) AllContents(ctx context.Context) ([]string, error) {
	var rows []string
	err := r.db.WithContext(ctx).Model(&domain.Poem{}).Pluck("content", &rows).Error
	return rows, err
}
