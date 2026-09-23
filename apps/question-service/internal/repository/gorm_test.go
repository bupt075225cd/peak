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

func TestMistakeRepoListByIDs(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()

	var mistakeIDs []uint64
	for _, stem := range []string{"a", "b", "c"} {
		q := &domain.Question{Subject: "math", StemText: stem}
		if err := repos.Question.Create(ctx, q); err != nil {
			t.Fatalf("create question: %v", err)
		}
		m := &domain.Mistake{UserID: userID, QuestionID: q.ID}
		if err := repos.Mistake.Create(ctx, m); err != nil {
			t.Fatalf("create mistake: %v", err)
		}
		mistakeIDs = append(mistakeIDs, m.ID)
	}

	// 其他用户的错题不应被查出。
	otherQuestion := &domain.Question{Subject: "math", StemText: "other"}
	if err := repos.Question.Create(ctx, otherQuestion); err != nil {
		t.Fatalf("create other question: %v", err)
	}
	foreign := &domain.Mistake{UserID: userID + 1, QuestionID: otherQuestion.ID}
	if err := repos.Mistake.Create(ctx, foreign); err != nil {
		t.Fatalf("create foreign mistake: %v", err)
	}

	list, err := repos.Mistake.ListByIDs(ctx, userID, []uint64{mistakeIDs[0], mistakeIDs[2], foreign.ID})
	if err != nil {
		t.Fatalf("ListByIDs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 own mistakes, got %d", len(list))
	}
	for i := range list {
		if list[i].Question == nil {
			t.Fatal("expected question to be preloaded")
		}
	}

	empty, err := repos.Mistake.ListByIDs(ctx, userID, nil)
	if err != nil {
		t.Fatalf("ListByIDs with empty ids: %v", err)
	}
	if empty != nil {
		t.Fatalf("expected nil result for empty ids, got %v", empty)
	}
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

	res, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("expected 1, got total=%d len=%d", res.Total, len(res.Items))
	}

	// 其他用户查询为空。
	other, _ := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: 999, Limit: 10})
	if other.Total != 0 || len(other.Items) != 0 {
		t.Fatal("expected empty for other user")
	}

	if err := repos.Mistake.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
}

// seedMistakesForFilter 预置三条错题，覆盖 不同学科/题型/知识点/来源，
// 其中第 2 条的题目来源为空（回退到错题记录来源），用于验证来源取值口径。
func seedMistakesForFilter(t *testing.T, repos *GormRepositories, userID uint64) {
	t.Helper()
	ctx := context.Background()

	seed := []struct {
		subject        string
		questionType   string
		stem           string
		kps            string
		source         string
		mistakeSource  string
	}{
		{"数学", "解答题", "已知二次函数 y=x^2 求顶点", `["二次函数"]`, "期中考试", "错题本"},
		{"数学", "选择题", "下列 Math 说法正确的是", `["函数"]`, "期中考试", ""},
		{"物理", "填空题", "一个物体做匀速直线运动", `["运动学"]`, "", "练习册 P32"},
	}
	for i, s := range seed {
		q := &domain.Question{
			Subject: s.subject, QuestionType: s.questionType, StemText: s.stem,
			KnowledgePoints: s.kps, Source: s.source,
		}
		if err := repos.Question.Create(ctx, q); err != nil {
			t.Fatalf("create question %d: %v", i, err)
		}
		m := &domain.Mistake{UserID: userID, QuestionID: q.ID, Source: s.mistakeSource}
		if err := repos.Mistake.Create(ctx, m); err != nil {
			t.Fatalf("create mistake %d: %v", i, err)
		}
	}
}

func TestMistakeListByUserKeyword(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()
	seedMistakesForFilter(t, repos, userID)

	cases := []struct {
		name    string
		keyword string
		want    int64
	}{
		{"命中题干且忽略大小写", "math", 1},
		{"多关键词全部命中", "二次函数 顶点", 1},
		{"多关键词任一不命中则为空", "二次函数 不存在的词", 0},
		{"命中知识点", "运动学", 1},
		{"命中题型", "填空题", 1},
		{"命中来源（题目来源为空时取错题来源）", "练习册", 1},
		{"命中学科（保持改造前的可搜行为）", "数学", 2},
		{"汉字子串命中", "函数", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{
				UserID: userID, Keyword: tc.keyword, Limit: 10,
			})
			if err != nil {
				t.Fatalf("ListByUser: %v", err)
			}
			if res.Total != tc.want {
				t.Fatalf("keyword %q: total = %d, want %d", tc.keyword, res.Total, tc.want)
			}
		})
	}
}

func TestMistakeListByUserSubjectAndSource(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()
	seedMistakesForFilter(t, repos, userID)

	math, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Subject: "数学", Limit: 10})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if math.Total != 2 {
		t.Fatalf("subject 数学: total = %d, want 2", math.Total)
	}

	// 来源优先取题目来源，题目来源为空时回退错题记录来源。
	midterm, _ := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Source: "期中考试", Limit: 10})
	if midterm.Total != 2 {
		t.Fatalf("source 期中考试: total = %d, want 2", midterm.Total)
	}
	workbook, _ := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Source: "练习册 P32", Limit: 10})
	if workbook.Total != 1 {
		t.Fatalf("source 练习册 P32: total = %d, want 1", workbook.Total)
	}

	// 学科与来源叠加。
	combo, _ := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{
		UserID: userID, Subject: "数学", Source: "期中考试", Limit: 10,
	})
	if combo.Total != 2 {
		t.Fatalf("subject+source: total = %d, want 2", combo.Total)
	}
}

func TestMistakeListByUserFacets(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()
	seedMistakesForFilter(t, repos, userID)

	res, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Limit: 10})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("total = %d, want 3", res.Total)
	}
	if res.SubjectCounts["数学"] != 2 || res.SubjectCounts["物理"] != 1 {
		t.Fatalf("subject counts = %v, want 数学:2 物理:1", res.SubjectCounts)
	}
	if res.SourceCounts["期中考试"] != 2 || res.SourceCounts["练习册 P32"] != 1 {
		t.Fatalf("source counts = %v, want 期中考试:2 练习册 P32:1", res.SourceCounts)
	}
	// 来源为空的历史数据不进入来源分面。
	if _, ok := res.SourceCounts[""]; ok {
		t.Fatal("empty source should not be reported in facets")
	}

	// 学科筛选不改变分面：其它学科仍需可见，才能切换筛选。
	filtered, _ := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Subject: "数学", Limit: 10})
	if filtered.Total != 2 {
		t.Fatalf("filtered total = %d, want 2", filtered.Total)
	}
	if filtered.SubjectCounts["物理"] != 1 || filtered.SubjectCounts["数学"] != 2 {
		t.Fatalf("facets should ignore subject filter, got %v", filtered.SubjectCounts)
	}
}

func TestMistakeListByUserPaginationIsStable(t *testing.T) {
	repos, userID := setupRepos(t)
	ctx := context.Background()
	seedMistakesForFilter(t, repos, userID)

	first, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Limit: 2})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	second, err := repos.Mistake.ListByUser(ctx, domain.MistakeQuery{UserID: userID, Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(first.Items) != 2 || len(second.Items) != 1 {
		t.Fatalf("page sizes = %d/%d, want 2/1", len(first.Items), len(second.Items))
	}

	// 两页之间不重复、不遗漏。
	seen := map[uint64]bool{}
	for _, m := range append(first.Items, second.Items...) {
		if seen[m.ID] {
			t.Fatalf("mistake %d appears on both pages", m.ID)
		}
		seen[m.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("paged through %d mistakes, want 3", len(seen))
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
