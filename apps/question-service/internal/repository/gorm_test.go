package repository

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm/logger"

	"peak/libs/domain"
)

func setupRepos(t *testing.T) (*GormRepositories, uint64) {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "repo.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 预置一个用户。
	u := &domain.User{Account: "u1", Name: "n1"}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return NewGormRepositories(db), u.ID
}

func TestQuestionRepoCRUD(t *testing.T) {
	repos, _ := setupRepos(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math", StemText: "1+1=?", Answer: "2"}
	if err := repos.Question.Create(ctx, q); err != nil {
		t.Fatalf("create: %v", err)
	}
	if q.ID == 0 {
		t.Fatal("expected id assigned")
	}

	got, err := repos.Question.Get(ctx, q.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StemText != "1+1=?" {
		t.Fatalf("unexpected stem: %s", got.StemText)
	}

	got.StemText = "updated"
	if err := repos.Question.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	list, total, err := repos.Question.List(ctx, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 item, got total=%d len=%d", total, len(list))
	}

	if err := repos.Question.Delete(ctx, q.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repos.Question.Get(ctx, q.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestMistakeRepoCRUD(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math", StemText: "x+1=2"}
	if err := repos.Question.Create(ctx, q); err != nil {
		t.Fatal(err)
	}

	m := &domain.Mistake{UserID: userID, QuestionID: q.ID, WrongReason: "careless"}
	if err := repos.Mistake.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repos.Mistake.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Question == nil || got.Question.ID != q.ID {
		t.Fatal("expected preloaded question")
	}

	got.WrongReason = "concept"
	if err := repos.Mistake.Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	list, total, err := repos.Mistake.ListByUser(ctx, userID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1, got total=%d len=%d", total, len(list))
	}

	// 其他用户查询为空。
	otherList, otherTotal, _ := repos.Mistake.ListByUser(ctx, 999, 0, 10)
	if otherTotal != 0 || len(otherList) != 0 {
		t.Fatal("expected empty for other user")
	}

	if err := repos.Mistake.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCategoryRepo(t *testing.T) {
	repos, _ := setupRepos(t)
	ctx := context.Background()

	c1 := &domain.Category{Name: "math", Type: "subject", SortOrder: 1}
	c2 := &domain.Category{Name: "func", Type: "knowledge", SortOrder: 2}
	if err := repos.Category.Create(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if err := repos.Category.Create(ctx, c2); err != nil {
		t.Fatal(err)
	}

	got, err := repos.Category.Get(ctx, c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "math" {
		t.Fatalf("unexpected name: %s", got.Name)
	}

	all, err := repos.Category.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}

	// 按类型过滤。
	subj, err := repos.Category.List(ctx, "subject")
	if err != nil {
		t.Fatal(err)
	}
	if len(subj) != 1 || subj[0].Name != "math" {
		t.Fatalf("expected 1 subject, got %d", len(subj))
	}

	got.Name = "math-updated"
	if err := repos.Category.Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	if err := repos.Category.Delete(ctx, c2.ID); err != nil {
		t.Fatal(err)
	}
}

func TestImageRepo(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math"}
	if err := repos.Question.Create(ctx, q); err != nil {
		t.Fatal(err)
	}
	m := &domain.Mistake{UserID: userID, QuestionID: q.ID}
	if err := repos.Mistake.Create(ctx, m); err != nil {
		t.Fatal(err)
	}

	img := &domain.Image{MistakeID: m.ID, StorageKey: "a/b.jpg", ImageType: "original"}
	if err := repos.Image.Create(ctx, img); err != nil {
		t.Fatal(err)
	}

	got, err := repos.Image.Get(ctx, img.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StorageKey != "a/b.jpg" {
		t.Fatalf("unexpected key: %s", got.StorageKey)
	}

	list, err := repos.Image.ListByMistake(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 image, got %d", len(list))
	}

	empty, err := repos.Image.ListByMistake(ctx, 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty, got %d", len(empty))
	}
}
