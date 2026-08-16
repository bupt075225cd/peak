package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	"gorm.io/gorm"

	"peak/libs/domain"
	"peak/libs/logger"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/provider"
)

// failingProvider 让 ExtractStructured 失败、ExtractDocument 可控，
// 用于覆盖 processDocument 的回退与错误分支。
type failingProvider struct {
	provider.MockProvider
	extractDocErr error
}

func (p *failingProvider) ExtractStructured(context.Context, []byte, string) (*provider.StructuredResult, error) {
	return nil, errors.New("structured unsupported")
}

func (p *failingProvider) ExtractDocument(ctx context.Context, data []byte, filename string) (*provider.DocumentResult, error) {
	if p.extractDocErr != nil {
		return nil, p.extractDocErr
	}
	return p.MockProvider.ExtractDocument(ctx, data, filename)
}

// failingStorage 让 Put 失败，覆盖 storeDocumentImages 的占位分支。
type failingStorage struct{}

func (f *failingStorage) Put(context.Context, string, []byte) error { return errors.New("put failed") }
func (f *failingStorage) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (f *failingStorage) Delete(context.Context, string) error { return nil }
func (f *failingStorage) PresignedURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("not implemented")
}

func setupServiceWithProvider(t *testing.T, prov provider.Provider) (*Service, storage.FileStorage) {
	t.Helper()
	dsn := t.TempDir() + "/test.db"
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
	return New(db, store, prov, logger.NewNop()), store
}

// buildTestDocx 构造含两个文本段落的简单 docx。
func buildTestDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>1. 已知集合 A={1,2,3}，求子集个数。</w:t></w:r></w:p>
    <w:p><w:r><w:t>2. 计算 lim(x→0) sin(x)/x。</w:t></w:r></w:p>
  </w:body>
</w:document>`
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(docXML))
	zw.Close()
	return buf.Bytes()
}

func TestProcessDocumentStructured(t *testing.T) {
	svc, store := setupServiceWithProvider(t, provider.NewMockProvider())
	ctx := context.Background()

	key := "original/paper.docx"
	if err := store.Put(ctx, key, buildTestDocx(t)); err != nil {
		t.Fatalf("put: %v", err)
	}

	res, err := svc.processDocument(ctx, 1, key)
	if err != nil {
		t.Fatalf("processDocument: %v", err)
	}
	if len(res.Questions) == 0 {
		t.Fatal("expected at least one question")
	}
}

func TestProcessDocumentFallback(t *testing.T) {
	svc, store := setupServiceWithProvider(t, &failingProvider{})
	ctx := context.Background()

	key := "original/paper.docx"
	if err := store.Put(ctx, key, buildTestDocx(t)); err != nil {
		t.Fatalf("put: %v", err)
	}

	res, err := svc.processDocument(ctx, 1, key)
	if err != nil {
		t.Fatalf("processDocument fallback: %v", err)
	}
	if len(res.Questions) == 0 {
		t.Fatal("expected at least one question from fallback")
	}
}

func TestProcessDocumentError(t *testing.T) {
	svc, store := setupServiceWithProvider(t, &failingProvider{extractDocErr: errors.New("parse fail")})
	ctx := context.Background()

	key := "original/paper.docx"
	if err := store.Put(ctx, key, buildTestDocx(t)); err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, err := svc.processDocument(ctx, 1, key); err == nil {
		t.Fatal("expected error when both structured and document fail")
	}
}

func TestProcessDocumentMissingFile(t *testing.T) {
	svc, _ := setupServiceWithProvider(t, provider.NewMockProvider())
	if _, err := svc.processDocument(context.Background(), 1, "original/missing.docx"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestStoreDocumentImages(t *testing.T) {
	svc, _ := setupServiceWithProvider(t, provider.NewMockProvider())
	ctx := context.Background()

	keys := svc.storeDocumentImages(ctx, 7, [][]byte{[]byte("a"), []byte("b")})
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != "document/task_7/img_1.jpg" || keys[1] != "document/task_7/img_2.jpg" {
		t.Fatalf("unexpected keys: %v", keys)
	}

	// 失败分支：Put 失败时应占位空字符串。
	failSvc := New(&gorm.DB{}, &failingStorage{}, provider.NewMockProvider(), logger.NewNop())
	keys = failSvc.storeDocumentImages(ctx, 8, [][]byte{[]byte("x")})
	if len(keys) != 1 || keys[0] != "" {
		t.Fatalf("expected empty placeholder on put failure, got %v", keys)
	}
}

// nonSubImage 不实现 SubImage 的 image 类型，用于覆盖 cropRect 兜底分支。
type nonSubImage struct {
	img *image.RGBA
}

func (n *nonSubImage) ColorModel() color.Model { return n.img.ColorModel() }
func (n *nonSubImage) Bounds() image.Rectangle { return n.img.Bounds() }
func (n *nonSubImage) At(x, y int) color.Color { return n.img.At(x, y) }

func TestCropRectFallback(t *testing.T) {
	src := &nonSubImage{img: image.NewRGBA(image.Rect(0, 0, 10, 10))}
	out := cropRect(src, image.Rect(2, 2, 5, 5))
	if out == nil {
		t.Fatal("expected non-nil crop")
	}
	if w, h := out.Bounds().Dx(), out.Bounds().Dy(); w != 3 || h != 3 {
		t.Fatalf("unexpected crop size: %dx%d", w, h)
	}
}

func TestClampInt(t *testing.T) {
	if clampInt(5, 0, 10) != 5 {
		t.Fatal("expected 5")
	}
	if clampInt(-1, 0, 10) != 0 {
		t.Fatal("expected lower clamp 0")
	}
	if clampInt(11, 0, 10) != 10 {
		t.Fatal("expected upper clamp 10")
	}
}
