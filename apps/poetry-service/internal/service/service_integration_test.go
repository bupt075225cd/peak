package service

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm/logger"

	"peak/apps/poetry-service/internal/importer"
	"peak/apps/poetry-service/internal/repository"
	"peak/libs/domain"
	"peak/libs/errors"
)

// setupService 以 SQLite 空库构建 Service 并导入真实语料。
func setupService(t *testing.T) *Service {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "svc.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, _, err := importer.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return New(repository.NewGormPoemRepository(db))
}

func TestDynastyStats(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	stats, err := svc.DynastyStats(ctx)
	if err != nil {
		t.Fatalf("DynastyStats: %v", err)
	}
	if len(stats) != len(domain.PoemDynasties) {
		t.Fatalf("stats length = %d, want %d", len(stats), len(domain.PoemDynasties))
	}
	// 顺序与计数：先秦 30 篇、宋最多、清 5 篇（按语料实际分布核对总数）。
	total := int64(0)
	for i, st := range stats {
		if st.Order != i || st.Dynasty != domain.PoemDynasties[i] {
			t.Errorf("stats[%d] = %+v, order/dynasty mismatch", i, st)
		}
		total += st.Count
	}
	if total != 124 {
		t.Errorf("total count = %d, want 124", total)
	}
	if stats[0].Count != 17 {
		t.Errorf("先秦 count = %d, want 17", stats[0].Count)
	}
}

func TestVolumeStats(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	stats, err := svc.VolumeStats(ctx)
	if err != nil {
		t.Fatalf("VolumeStats: %v", err)
	}
	if len(stats) < 6 {
		t.Fatalf("stats = %d rows, want >= 6 (含非标准册数)", len(stats))
	}
	total := int64(0)
	for _, st := range stats {
		total += st.Inclass + st.Outline + st.Other
	}
	if total != 124 {
		t.Errorf("total = %d, want 124", total)
	}
	// 旧版未收录篇目（陈涉世家）归入 Other。
	foundOther := false
	for _, st := range stats[6:] {
		if st.Other > 0 {
			foundOther = true
		}
	}
	if !foundOther {
		t.Errorf("non-standard volume with Other count expected, stats=%+v", stats)
	}
}

func TestAuthorsAndWordFreq(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	authors, err := svc.Authors(ctx, 5)
	if err != nil || len(authors) != 5 {
		t.Fatalf("Authors top5 = %d, %v", len(authors), err)
	}
	if authors[0].Count < authors[4].Count {
		t.Errorf("author ranking not descending: %+v", authors)
	}

	freq, total, err := svc.WordFreq(ctx, 50)
	if err != nil {
		t.Fatalf("WordFreq: %v", err)
	}
	if len(freq) != 50 || total == 0 {
		t.Fatalf("WordFreq top50 = %d words, total=%d", len(freq), total)
	}
	// 缓存后再次请求一致。
	freq2, total2, _ := svc.WordFreq(ctx, 10)
	if total2 != total || len(freq2) != 10 {
		t.Errorf("cached WordFreq mismatch: %d/%d", len(freq2), total2)
	}
	// top=0 返回全部。
	all, _, _ := svc.WordFreq(ctx, 0)
	if len(all) < len(freq) {
		t.Errorf("full freq should be >= top50, got %d", len(all))
	}
}

func TestGetPoemNotFound(t *testing.T) {
	svc := setupService(t)
	if _, err := svc.GetPoem(context.Background(), 99999); errors.CodeOf(err) != errors.CodeNotFound {
		t.Errorf("GetPoem missing should be CodeNotFound, got %v", err)
	}
}

func TestListAndRelated(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	items, total, err := svc.ListPoems(ctx, repository.Filter{Dynasty: "宋", Limit: 5})
	if err != nil || total == 0 || len(items) != 5 {
		t.Fatalf("ListPoems 宋: %d items, total=%d, err=%v", len(items), total, err)
	}

	poem, err := svc.GetPoem(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("GetPoem: %v", err)
	}
	related, err := svc.RelatedPoems(ctx, poem, 4)
	if err != nil {
		t.Fatalf("RelatedPoems: %v", err)
	}
	for _, r := range related {
		if r.Author != poem.Author && r.Dynasty != poem.Dynasty {
			t.Errorf("related poem %q neither same author nor dynasty", r.Title)
		}
	}
}
