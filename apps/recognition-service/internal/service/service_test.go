package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/errors"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/provider"
)

func setupService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	// 使用文件型 SQLite，避免 :memory: 每个连接独立导致的并发问题。
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := domain.OpenDB(domain.DialectSQLite, dsn, 1)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	svc := New(db, store, provider.NewMockProvider(), logger.NewNop())
	return svc, db
}

func TestCreateAndGetTask(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	key := "original/test.jpg"
	task, err := svc.CreateTask(ctx, 1, key)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != domain.TaskPending {
		t.Fatalf("expected pending, got %s", task.Status)
	}

	got, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("id mismatch")
	}
}

func TestProcessTaskSuccess(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	key := "original/p.jpg"
	if err := svc.storage.Put(ctx, key, []byte("fake-image-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}

	task, err := svc.CreateTask(ctx, 1, key)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// 等待异步处理完成。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.GetTask(ctx, task.ID)
		if got.Status == domain.TaskSuccess {
			if got.ResultJSON == "" {
				t.Fatal("expected result json")
			}
			return
		}
		if got.Status == domain.TaskFailed {
			t.Fatalf("unexpected failure: %s", got.ErrorMessage)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timeout waiting for task success")
}

func TestProcessTaskReadImageFailed(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	// 图片不存在，process 应标记 failed。
	task, err := svc.CreateTask(ctx, 1, "original/missing.jpg")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.GetTask(ctx, task.ID)
		if got.Status == domain.TaskFailed {
			if got.ErrorMessage == "" {
				t.Fatal("expected error message")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timeout waiting for task failure")
}

func TestGetTaskNotFound(t *testing.T) {
	svc, _ := setupService(t)
	_, err := svc.GetTask(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestRetryTask(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	// 创建一个必然失败的任务（图片缺失）。
	task, err := svc.CreateTask(ctx, 1, "original/none.jpg")
	if err != nil {
		t.Fatal(err)
	}

	// 等待失败。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.GetTask(ctx, task.ID)
		if got.Status == domain.TaskFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 重试。
	if err := svc.RetryTask(ctx, task.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got, _ := svc.GetTask(ctx, task.ID)
	if got.RetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", got.RetryCount)
	}
	if got.Status != domain.TaskPending && got.Status != domain.TaskFailed {
		t.Fatalf("expected pending or failed after retry, got %s", got.Status)
	}
}

func TestRetryTaskNotFound(t *testing.T) {
	svc, _ := setupService(t)
	err := svc.RetryTask(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.CodeOf(err) != errors.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", errors.CodeOf(err))
	}
}

func TestErasedKeyAndItoa(t *testing.T) {
	if erasedKey(0) != "erased/task_0.jpg" {
		t.Fatalf("unexpected key: %s", erasedKey(0))
	}
	if erasedKey(42) != "erased/task_42.jpg" {
		t.Fatalf("unexpected key: %s", erasedKey(42))
	}
	if itoa(0) != "0" {
		t.Fatalf("itoa(0) = %s", itoa(0))
	}
	if itoa(12345) != "12345" {
		t.Fatalf("itoa(12345) = %s", itoa(12345))
	}
}
