package repository

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm/logger"

	"peak/libs/domain"
)

func setupDB(t *testing.T) PoemRepository {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "repo.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seed := []domain.Poem{
		{Title: "关雎", Dynasty: "先秦", DynastyOrder: 0, Author: "佚名", Volume: "八年级下册", Category: "课内", Content: "关关雎鸠", WordCount: 4},
		{Title: "蒹葭", Dynasty: "先秦", DynastyOrder: 0, Author: "佚名", Volume: "八年级下册", Category: "课外", Content: "蒹葭苍苍", WordCount: 4},
		{Title: "观沧海", Dynasty: "汉", DynastyOrder: 1, Author: "曹操", Volume: "七年级上册", Category: "课内", Content: "东临碣石", WordCount: 4},
		{Title: "望岳", Dynasty: "唐", DynastyOrder: 5, Author: "杜甫", Volume: "七年级下册", Category: "课内", Content: "岱宗夫如何", WordCount: 5},
		{Title: "旧版篇目", Dynasty: "唐", DynastyOrder: 5, Author: "司马迁", Volume: "（统编版未收录）", Category: "", Content: "参读", WordCount: 2},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed poem: %v", err)
		}
	}
	return NewGormPoemRepository(db)
}

func TestListFilters(t *testing.T) {
	repo := setupDB(t)
	ctx := context.Background()

	// 不筛选：按朝代序排序。
	items, total, err := repo.List(ctx, Filter{Limit: 10})
	if err != nil || total != 5 {
		t.Fatalf("List all: total=%d err=%v", total, err)
	}
	if items[0].Title != "关雎" || items[4].Title != "旧版篇目" {
		t.Errorf("List order unexpected: %v", titles(items))
	}

	// 朝代筛选。
	_, total, _ = repo.List(ctx, Filter{Dynasty: "先秦", Limit: 10})
	if total != 2 {
		t.Errorf("List by dynasty: total=%d want 2", total)
	}

	// 分类 + 册数组合。
	items, total, _ = repo.List(ctx, Filter{Volume: "八年级下册", Category: "课外", Limit: 10})
	if total != 1 || items[0].Title != "蒹葭" {
		t.Errorf("List volume+category: %v %d", titles(items), total)
	}

	// 关键词匹配正文。
	items, total, _ = repo.List(ctx, Filter{Keyword: "岱宗", Limit: 10})
	if total != 1 || items[0].Title != "望岳" {
		t.Errorf("List keyword: %v %d", titles(items), total)
	}

	// 关键词匹配篇名。
	_, total, _ = repo.List(ctx, Filter{Keyword: "关雎", Limit: 10})
	if total != 1 {
		t.Errorf("List keyword by title: %d", total)
	}

	// 分页。
	items, total, _ = repo.List(ctx, Filter{Offset: 1, Limit: 2})
	if len(items) != 2 || total != 5 || items[0].Title != "蒹葭" {
		t.Errorf("List paginated: %v total=%d", titles(items), total)
	}
}

func TestGetByIDAndRelated(t *testing.T) {
	repo := setupDB(t)
	ctx := context.Background()

	missing, err := repo.GetByID(ctx, 999)
	if err != nil || missing != nil {
		t.Fatalf("GetByID missing: %v, %v", missing, err)
	}

	p, err := repo.GetByID(ctx, 3) // 观沧海
	if err != nil || p == nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p.Author != "曹操" {
		t.Errorf("GetByID author = %q", p.Author)
	}

	// 相关推荐：同作者优先（无），同朝代（汉：仅自身外无），降级到其他结果数量正确。
	related, err := repo.Related(ctx, p, 4)
	if err != nil {
		t.Fatalf("Related: %v", err)
	}
	if len(related) != 0 {
		t.Errorf("Related for sole dynasty poem should be empty, got %v", titles(related))
	}

	dufu, _ := repo.GetByID(ctx, 4) // 望岳
	related, _ = repo.Related(ctx, dufu, 4)
	// 同作者：无；同朝代（唐）：旧版篇目。推荐 1 条。
	if len(related) != 1 || related[0].Title != "旧版篇目" {
		t.Errorf("Related of 望岳 = %v, want [旧版篇目]", titles(related))
	}

	anon, _ := repo.GetByID(ctx, 1) // 关雎
	related, _ = repo.Related(ctx, anon, 1)
	// 同作者（佚名）：蒹葭，limit=1 截断。
	if len(related) != 1 || related[0].Title != "蒹葭" {
		t.Errorf("Related of 关雎 = %v, want [蒹葭]", titles(related))
	}
}

func TestStatsQueries(t *testing.T) {
	repo := setupDB(t)
	ctx := context.Background()

	counts, err := repo.CountByDynasty(ctx)
	if err != nil {
		t.Fatalf("CountByDynasty: %v", err)
	}
	if counts["先秦"] != 2 || counts["汉"] != 1 || counts["唐"] != 2 {
		t.Errorf("CountByDynasty = %v", counts)
	}

	vols, err := repo.CountByVolume(ctx)
	if err != nil {
		t.Fatalf("CountByVolume: %v", err)
	}
	if len(vols) != 5 { // (八下,课内)(八下,课外)(七上,课内)(七下,课内)(未收录,空)
		t.Errorf("CountByVolume rows = %d, want 5", len(vols))
	}

	authors, err := repo.AuthorRanking(ctx, 1)
	if err != nil {
		t.Fatalf("AuthorRanking: %v", err)
	}
	if len(authors) != 1 || authors[0].Count < 2 {
		t.Errorf("AuthorRanking top1 = %+v, want 佚名×2", authors)
	}
	all, _ := repo.AuthorRanking(ctx, 0)
	if len(all) != 4 {
		t.Errorf("AuthorRanking all = %d, want 4", len(all))
	}

	contents, err := repo.AllContents(ctx)
	if err != nil || len(contents) != 5 {
		t.Errorf("AllContents = %d, %v", len(contents), err)
	}
}

func titles(poems []domain.Poem) []string {
	out := make([]string, 0, len(poems))
	for _, p := range poems {
		out = append(out, p.Title)
	}
	return out
}
