package service

import (
	"context"
	"encoding/json"
	"fmt"
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
// VLM 返回的每个子图各自存储为独立 SVG key 并写入 redraw_figures，
// 同时保留图号（图1/图2），供导出时把标注补回配图。
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
	if len(result.RedrawFigures) != 2 {
		t.Fatalf("expected 2 redraw figures, got %v", result.RedrawFigures)
	}
	// 每个子图独立存储且可读回，内容为合法 SVG 文档；图号按子图顺序保留。
	for i, fig := range result.RedrawFigures {
		data, err := store.Get(ctx, fig.Key)
		if err != nil {
			t.Fatalf("svg key not stored %s: %v", fig.Key, err)
		}
		if !strings.HasPrefix(string(data), "<svg ") {
			t.Fatalf("unexpected stored content for %s: %q", fig.Key, data)
		}
		if !strings.Contains(string(data), "viewBox") {
			t.Fatalf("expected viewBox in %s: %q", fig.Key, data)
		}
		if want := fmt.Sprintf("图%d", i+1); fig.Label != want {
			t.Fatalf("figure %d label = %q, want %q", i, fig.Label, want)
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
	if len(result.RedrawFigures) != 0 {
		t.Fatalf("expected no redraw figures without engine, got %v", result.RedrawFigures)
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

// scriptedSpecProvider 按脚本顺序返回几何描述（超出后重复最后一个），
// 用于驱动"校验失败回喂修正"与"渲染失败告警"分支。
type scriptedSpecProvider struct {
	provider.Provider
	specs []string
	calls int
}

func (f *scriptedSpecProvider) ExtractGeometrySpec(_ context.Context, _ []byte, _, _ string) (string, error) {
	i := f.calls
	if i >= len(f.specs) {
		i = len(f.specs) - 1
	}
	f.calls++
	return f.specs[i], nil
}

// runGeometryTask 用给定 provider 跑一次完整图片识别（含几何重绘），返回解析后的结果。
func runGeometryTask(t *testing.T, prov provider.Provider) RecognitionResult {
	t.Helper()
	ctx := context.Background()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "geo.db"), 1)
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
	svc := New(db, store, prov, logger.NewNop(), WithGeometryRender(true, 3))

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
	return result
}

// TestProcessImageRedrawFeedsValidationIssuesBack 验证结构校验失败时：
// 把问题清单回喂 VLM 修正后重试，第二轮通过且 attempts 记录实际轮数。
func TestProcessImageRedrawFeedsValidationIssuesBack(t *testing.T) {
	// 首轮：线段引用了不存在的点 B（结构校验失败）。
	bad := `{"title":"图1","canvas":{"width":100,"height":100},` +
		`"points":[{"name":"A","x":10,"y":10}],"segments":[{"from":"A","to":"B"}]}`
	// 第二轮：修正为合法的双子图描述。
	good := `{"panels":[` +
		`{"title":"图1","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"A","x":15,"y":60},{"name":"B","x":85,"y":60},{"name":"C","x":50,"y":20}],` +
		`"segments":[{"from":"A","to":"B"},{"from":"A","to":"C"},{"from":"B","to":"C"}]},` +
		`{"title":"图2","canvas":{"width":100,"height":80},` +
		`"points":[{"name":"O","x":50,"y":40},{"name":"P","x":80,"y":40}],` +
		`"circles":[{"center":"O","through":"P"}]}` +
		`]}`
	result := runGeometryTask(t, &scriptedSpecProvider{
		Provider: provider.NewMockProvider(), specs: []string{bad, good},
	})

	if len(result.RedrawFigures) != 2 {
		t.Fatalf("expected 2 redraw figures after correction, got %v", result.RedrawFigures)
	}
	if result.RedrawReport == nil {
		t.Fatal("expected redraw report")
	}
	if result.RedrawReport.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", result.RedrawReport.Attempts)
	}
	if !result.RedrawReport.Consistent {
		t.Fatalf("expected consistent after correction, got %+v", result.RedrawReport)
	}
	if result.Warning != "" {
		t.Fatalf("expected no warning after correction, got %q", result.Warning)
	}
}

// TestProcessImageRedrawFailureAddsWarning 验证渲染持续失败时：
// 不产出重绘 key，降级为 warning，不影响主识别结果。
func TestProcessImageRedrawFailureAddsWarning(t *testing.T) {
	result := runGeometryTask(t, &scriptedSpecProvider{
		Provider: provider.NewMockProvider(), specs: []string{"not-a-json"},
	})
	if len(result.RedrawFigures) != 0 {
		t.Fatalf("expected no redraw figures on persistent failure, got %v", result.RedrawFigures)
	}
	if result.RedrawReport != nil {
		t.Fatalf("expected no redraw report on failure, got %+v", result.RedrawReport)
	}
	if !strings.Contains(result.Warning, "几何重绘失败") {
		t.Fatalf("expected redraw failure warning, got %q", result.Warning)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abcd..." {
		t.Fatalf("truncate long string = %q", got)
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Fatalf("truncate short string = %q", got)
	}
}

func TestFormatValidateIssues(t *testing.T) {
	out := formatValidateIssues([]string{"图1：点 B 未定义"})
	if !strings.Contains(out, "结构问题") || !strings.Contains(out, "图1：点 B 未定义") {
		t.Fatalf("unexpected output: %s", out)
	}

	// 超过 10 条时截断并提示省略。
	many := make([]string, 0, 15)
	for i := 1; i <= 15; i++ {
		many = append(many, fmt.Sprintf("问题%d", i))
	}
	out = formatValidateIssues(many)
	if !strings.Contains(out, "其余问题省略") {
		t.Fatalf("expected ellipsis note: %s", out)
	}
	if strings.Contains(out, "问题11") {
		t.Fatalf("issues beyond the 10th should be omitted: %s", out)
	}
}


