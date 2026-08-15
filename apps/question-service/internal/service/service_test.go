package service

import (
	"context"
	"path/filepath"
	"testing"

	"peak/libs/domain"
	"peak/libs/errors"

	"peak/apps/question-service/internal/repository"
)

func setupService(t *testing.T) *Service {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := domain.OpenDB(domain.DialectSQLite, dsn, 1)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(repository.NewGormRepositories(db))
}

func TestCreateAndGetQuestion(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	q := &domain.Question{
		Subject:      "数学",
		StemText:     "已知二次函数 y = x^2 - 2x - 3，求顶点坐标。",
		StemFormula:  `{"latex":"y = x^2 - 2x - 3"}`,
		Answer:       "顶点 (1, -4)",
		Analysis:     "配方：y = (x-1)^2 - 4",
		Difficulty:   3,
		QuestionType: "解答题",
	}
	if err := svc.CreateQuestion(ctx, q); err != nil {
		t.Fatalf("create question: %v", err)
	}
	if q.ID == 0 {
		t.Fatal("expected question id")
	}

	got, err := svc.GetQuestion(ctx, q.ID)
	if err != nil {
		t.Fatalf("get question: %v", err)
	}
	if got.StemText != q.StemText {
		t.Fatalf("stem mismatch: %s", got.StemText)
	}
}

func TestGetQuestionNotFound(t *testing.T) {
	svc := setupService(t)
	_, err := svc.GetQuestion(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for missing question")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestUpdateQuestionNotFound(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	err := svc.UpdateQuestion(ctx, &domain.Question{ID: 9999})
	if err == nil {
		t.Fatal("expected error for missing question")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestUpdateQuestionSuccess(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math", StemText: "before"}
	if err := svc.CreateQuestion(ctx, q); err != nil {
		t.Fatal(err)
	}

	q.StemText = "after"
	if err := svc.UpdateQuestion(ctx, q); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := svc.GetQuestion(ctx, q.ID)
	if got.StemText != "after" {
		t.Fatalf("expected after, got %s", got.StemText)
	}
}

func TestDeleteQuestion(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math"}
	if err := svc.CreateQuestion(ctx, q); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteQuestion(ctx, q.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetQuestion(ctx, q.ID); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestListQuestions(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := svc.CreateQuestion(ctx, &domain.Question{Subject: "math"}); err != nil {
			t.Fatal(err)
		}
	}
	list, total, err := svc.ListQuestions(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(list) != 3 {
		t.Fatalf("expected 3, got total=%d len=%d", total, len(list))
	}
}

func TestCreateMistakeRequiresUser(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	err := svc.CreateMistake(ctx, &domain.Mistake{QuestionID: 1})
	if err == nil {
		t.Fatal("expected error for missing user_id")
	}
	if errors.CodeOf(err) != errors.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %d", errors.CodeOf(err))
	}
}

func TestCreateMistakeRequiresQuestion(t *testing.T) {
	svc := setupService(t)
	err := svc.CreateMistake(context.Background(), &domain.Mistake{UserID: 1})
	if err == nil {
		t.Fatal("expected error for missing question_id")
	}
	if errors.CodeOf(err) != errors.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %d", errors.CodeOf(err))
	}
}

func TestMistakeCRUD(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	q := &domain.Question{Subject: "math"}
	if err := svc.CreateQuestion(ctx, q); err != nil {
		t.Fatal(err)
	}

	m := &domain.Mistake{UserID: 1, QuestionID: q.ID, WrongReason: "careless"}
	if err := svc.CreateMistake(ctx, m); err != nil {
		t.Fatalf("create mistake: %v", err)
	}

	got, err := svc.GetMistake(ctx, m.ID)
	if err != nil {
		t.Fatalf("get mistake: %v", err)
	}
	if got.WrongReason != "careless" {
		t.Fatalf("unexpected reason: %s", got.WrongReason)
	}

	got.WrongReason = "concept"
	if err := svc.UpdateMistake(ctx, got); err != nil {
		t.Fatalf("update mistake: %v", err)
	}

	list, total, err := svc.ListMistakes(ctx, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1, got total=%d len=%d", total, len(list))
	}

	if err := svc.DeleteMistake(ctx, m.ID); err != nil {
		t.Fatalf("delete mistake: %v", err)
	}
}

func TestGetMistakeNotFound(t *testing.T) {
	svc := setupService(t)
	_, err := svc.GetMistake(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestUpdateMistakeNotFound(t *testing.T) {
	svc := setupService(t)
	err := svc.UpdateMistake(context.Background(), &domain.Mistake{ID: 9999})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestCategoryCRUD(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	c := &domain.Category{Name: "math", Type: "subject"}
	if err := svc.CreateCategory(ctx, c); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListCategories(ctx, "subject")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "math" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// 过滤不匹配的类型。
	empty, err := svc.ListCategories(ctx, "tag")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty, got %d", len(empty))
	}
}
