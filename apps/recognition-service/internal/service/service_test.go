package service

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
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

// makeTestJPEG 生成一张指定尺寸的纯色 JPEG 图片，便于裁剪测试。
func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

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

// TestProcessImageStoresGeometryKey 验证单图上传场景下，
// 识别结果中应携带一张“裁剪后的几何图” storage key（而非整张原图）。
// mock provider 返回的 bbox 为右下角 1/4 区域，裁剪结果应能正常解码且尺寸约为原图一半。
func TestProcessImageStoresGeometryKey(t *testing.T) {
	svc, db := setupService(t)
	ctx := context.Background()

	key := "original/geo.jpg"
	original := makeTestJPEG(t, 200, 200)
	if err := svc.storage.Put(ctx, key, original); err != nil {
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

	var got *domain.RecognitionTask
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ = svc.GetTask(ctx, task.ID)
		if got.Status == domain.TaskSuccess {
			break
		}
		if got.Status == domain.TaskFailed {
			t.Fatalf("unexpected failure: %s", got.ErrorMessage)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got == nil || got.Status != domain.TaskSuccess {
		t.Fatal("timeout waiting for task success")
	}
	if got.ResultJSON == "" {
		t.Fatal("expected result json")
	}
	var result RecognitionResult
	if err := json.Unmarshal([]byte(got.ResultJSON), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.GeometryKeys) == 0 {
		t.Fatalf("expected at least one geometry key, got none (result=%+v)", result)
	}
	geoKey := result.GeometryKeys[0]
	// 验证存储里确有这张图，且是裁剪后的子图（尺寸约为原图一半），而非整张原图。
	data, err := svc.storage.Get(ctx, geoKey)
	if err != nil {
		t.Fatalf("geometry key not stored: %v", err)
	}
	cropped, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("stored geometry image is not decodable: %v", err)
	}
	// mock bbox 为右下 1/4（x=0.5,y=0.5,w=0.5,h=0.5），外扩 10% 后被 clamp：
	// x = 0.5 - 0.1 = 0.4，w = min(0.7, 1-0.4) = 0.6，裁剪结果约 120x120。
	if w := cropped.Bounds().Dx(); w < 115 || w > 125 {
		t.Fatalf("unexpected cropped width: %d (expected ~120 after padding)", w)
	}
	if h := cropped.Bounds().Dy(); h < 115 || h > 125 {
		t.Fatalf("unexpected cropped height: %d (expected ~120 after padding)", h)
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
