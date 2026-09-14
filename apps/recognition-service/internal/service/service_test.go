package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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
	svc, db := setupService(t)
	ctx := context.Background()

	key := "original/p.jpg"
	if err := svc.storage.Put(ctx, key, []byte("fake-image-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}

	img := &domain.Image{StorageKey: key, ImageType: domain.ImageTypeOriginal}
	if err := db.Create(img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}

	task, err := svc.CreateTask(ctx, img.ID, key)
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

// waitTask 轮询任务到成功/失败。
func waitTask(t *testing.T, svc *Service, ctx context.Context, id uint64) *domain.RecognitionTask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := svc.GetTask(ctx, id)
		if got.Status == domain.TaskSuccess || got.Status == domain.TaskFailed {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timeout waiting for task")
	return nil
}

// fakeMultiPanelSpecProvider 在 mock 基础上返回双子图几何描述，
// 用于验证多子图（图1/图2）各自独立渲染与存储。
type fakeMultiPanelSpecProvider struct {
	provider.Provider
}

func (f *fakeMultiPanelSpecProvider) ExtractGeometrySpec(_ context.Context, _ []byte, _, _ string) (string, error) {
	return `{"panels":[` +
		`{"title":"图1","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"A","x":15,"y":60},{"name":"B","x":85,"y":60},{"name":"C","x":50,"y":20}],` +
		`"segments":[{"from":"A","to":"B"},{"from":"A","to":"C"},{"from":"B","to":"C"}],` +
		`"polygons":[{"points":["A","B","C"]}]},` +
		`{"title":"图2","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"O","x":50,"y":40},{"name":"P","x":80,"y":40},{"name":"Q","x":50,"y":70}],` +
		`"segments":[{"from":"O","to":"P"},{"from":"O","to":"Q"}],` +
		`"circles":[{"center":"O","through":"P"}]}` +
		`]}`, nil
}

// TestProcessImageMathRedrawProducesMultiSVG 验证单图识别时：
// 数学题 + 含几何图（mock 返回 bbox）且启用了内置几何渲染时，
// VLM 返回的每个子图各自存储为独立 SVG key 并写入 redraw_svg_keys。
func TestProcessImageMathRedrawProducesMultiSVG(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "redraw.db")
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
	svc := New(db, store, &fakeMultiPanelSpecProvider{Provider: provider.NewMockProvider()},
		logger.NewNop(), WithGeometryRender(true, 3))

	key := "original/geo.jpg"
	if err := store.Put(ctx, key, []byte("fake-image-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}
	img := &domain.Image{StorageKey: key, ImageType: domain.ImageTypeOriginal}
	if err := db.Create(img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	task, err := svc.CreateTask(ctx, img.ID, key)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	got := waitTask(t, svc, ctx, task.ID)
	if got.Status != domain.TaskSuccess {
		t.Fatalf("expected success, got %s: %s", got.Status, got.ErrorMessage)
	}
	var result RecognitionResult
	if err := json.Unmarshal([]byte(got.ResultJSON), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.RedrawSVGKeys) != 2 {
		t.Fatalf("expected 2 redraw svg keys, got %v", result.RedrawSVGKeys)
	}
	// 每个子图独立存储且可读回，内容为合法 SVG 文档。
	for _, k := range result.RedrawSVGKeys {
		data, err := store.Get(ctx, k)
		if err != nil {
			t.Fatalf("svg key not stored %s: %v", k, err)
		}
		if !strings.HasPrefix(string(data), "<svg ") {
			t.Fatalf("unexpected stored content for %s: %q", k, data)
		}
		if !strings.Contains(string(data), "viewBox") {
			t.Fatalf("expected viewBox in %s: %q", k, data)
		}
	}
	if result.RedrawReport == nil || !result.RedrawReport.Consistent {
		t.Fatalf("expected consistent redraw report, got %+v", result.RedrawReport)
	}
	if result.RedrawReport.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", result.RedrawReport.Attempts)
	}
}

// TestProcessImageNoRedrawEngineSkips 验证未启用几何重绘（默认 mock 服务）时，
// 流程正常完成、不产出重绘 key，也不产生裁剪子图存储。
func TestProcessImageNoRedrawEngineSkips(t *testing.T) {
	svc, db := setupService(t)
	ctx := context.Background()

	key := "original/no.svg"
	if err := svc.storage.Put(ctx, key, []byte("fake-image-bytes")); err != nil {
		t.Fatalf("put: %v", err)
	}
	img := &domain.Image{StorageKey: key, ImageType: domain.ImageTypeOriginal}
	if err := db.Create(img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	task, err := svc.CreateTask(ctx, img.ID, key)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	got := waitTask(t, svc, ctx, task.ID)
	if got.Status != domain.TaskSuccess {
		t.Fatalf("expected success, got %s: %s", got.Status, got.ErrorMessage)
	}
	var result RecognitionResult
	if err := json.Unmarshal([]byte(got.ResultJSON), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.RedrawSVGKeys) != 0 {
		t.Fatalf("expected no redraw keys without engine, got %v", result.RedrawSVGKeys)
	}
}

func TestProcessTaskReadImageFailed(t *testing.T) {
	svc, db := setupService(t)
	ctx := context.Background()

	// 图片文件不存在，process 应标记 failed。
	img := &domain.Image{StorageKey: "original/missing.jpg", ImageType: domain.ImageTypeOriginal}
	if err := db.Create(img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}

	task, err := svc.CreateTask(ctx, img.ID, "original/missing.jpg")
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

func TestItoa(t *testing.T) {
	if itoa(0) != "0" {
		t.Fatalf("itoa(0) = %s", itoa(0))
	}
	if itoa(12345) != "12345" {
		t.Fatalf("itoa(12345) = %s", itoa(12345))
	}
}


