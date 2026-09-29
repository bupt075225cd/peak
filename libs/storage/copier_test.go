package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// fakeCopier 记录 CopyFrom 参数的桩实现，验证 ObjectCopier 优先路径。
type fakeCopier struct {
	FileStorage
	srcBucket, srcKey, dstKey string
	called                    bool
}

func (f *fakeCopier) CopyFrom(_ context.Context, srcBucket, srcKey, dstKey string) error {
	f.called = true
	f.srcBucket, f.srcKey, f.dstKey = srcBucket, srcKey, dstKey
	return nil
}

// TestCopierPrefersObjectCopier 目标存储实现 ObjectCopier 时应走服务端拷贝，
// 不读源存储、不落应用进程内存。
func TestCopierPrefersObjectCopier(t *testing.T) {
	dst := &fakeCopier{FileStorage: &LocalStorage{}}
	c := NewCopier(dst, nil, "src-bucket")
	if err := c.Copy(context.Background(), "transient/a.svg", "committed/a.svg"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if !dst.called {
		t.Fatal("CopyFrom should be called on ObjectCopier destination")
	}
	if dst.srcBucket != "src-bucket" || dst.srcKey != "transient/a.svg" || dst.dstKey != "committed/a.svg" {
		t.Fatalf("unexpected args: %+v", dst)
	}
}

// TestCopierFallbackGetPut 目标存储不支持 ObjectCopier 时（本地磁盘）回退为
// 源读+目标写，可跨根目录复制。
func TestCopierFallbackGetPut(t *testing.T) {
	src, err := NewLocalStorage(filepath.Join(t.TempDir(), "recognition"))
	if err != nil {
		t.Fatal(err)
	}
	dst, err := NewLocalStorage(filepath.Join(t.TempDir(), "question"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := src.Put(ctx, "transient/original/x.jpg", []byte("image-bytes")); err != nil {
		t.Fatal(err)
	}
	c := NewCopier(dst, src, "")
	if err := c.Copy(ctx, "transient/original/x.jpg", "committed/original/x.jpg"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	data, err := dst.Get(ctx, "committed/original/x.jpg")
	if err != nil || string(data) != "image-bytes" {
		t.Fatalf("dst get: %q %v", data, err)
	}
	// 源对象保持不变（提交拷贝不移动，transient/ 由生命周期规则清理）。
	if _, err := src.Get(ctx, "transient/original/x.jpg"); err != nil {
		t.Fatalf("src should remain: %v", err)
	}
}

// TestCopierFallbackMissingSource 源对象不存在时错误透传为 ErrNotFound。
func TestCopierFallbackMissingSource(t *testing.T) {
	src, _ := NewLocalStorage(t.TempDir())
	dst, _ := NewLocalStorage(t.TempDir())
	c := NewCopier(dst, src, "")
	err := c.Copy(context.Background(), "missing", "committed/missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestCopySource 验证 CopySource 的 URL 编码规则。
func TestCopySource(t *testing.T) {
	cases := []struct{ bucket, key, want string }{
		{"src", "transient/geometry/task_1.svg", "src/transient/geometry/task_1.svg"},
		{"src", "a b/c+d.png", "src/a%20b/c+d.png"},
	}
	for _, c := range cases {
		if got := copySource(c.bucket, c.key); got != c.want {
			t.Fatalf("copySource(%q, %q) = %q, want %q", c.bucket, c.key, got, c.want)
		}
	}
}
