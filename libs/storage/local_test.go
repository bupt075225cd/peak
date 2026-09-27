package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalStoragePutGetDelete(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	ctx := context.Background()

	key := "a/b/c.txt"
	if err := store.Put(ctx, key, []byte("hello")); err != nil {
		t.Fatalf("put: %v", err)
	}

	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected data: %s", data)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := store.Get(ctx, key); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLocalStorageGetMissing(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	if _, err := store.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalStorageDeleteMissing(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	// 删除不存在的对象应不返回错误。
	if err := store.Delete(context.Background(), "not-exist"); err != nil {
		t.Fatalf("delete missing should not error, got %v", err)
	}
}

func TestLocalStoragePresignedURL(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	url, err := store.PresignedURL(context.Background(), "a/b.png", time.Minute)
	if err != nil {
		t.Fatalf("presigned url: %v", err)
	}
	if url != "a/b.png" {
		t.Fatalf("expected key returned, got %s", url)
	}
}

func TestLocalStoragePutCreatesParentDir(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	key := "deep/nested/path/file.bin"
	if err := store.Put(context.Background(), key, []byte("x")); err != nil {
		t.Fatalf("put: %v", err)
	}
	// 验证父目录确实被创建且文件存在。
	if _, err := os.Stat(filepath.Join(root, "deep/nested/path/file.bin")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
}

func TestLocalStorageGetErrNotFoundType(t *testing.T) {
	store, _ := NewLocalStorage(t.TempDir())
	_, err := store.Get(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
	var ne *NotFoundError
	if !errors.As(err, &ne) {
		t.Fatalf("expected errors.As to NotFoundError, got %T", err)
	}
}

func TestNotFoundError(t *testing.T) {
	e := &NotFoundError{}
	if e.Error() != "object not found" {
		t.Fatalf("unexpected message: %s", e.Error())
	}
}

func TestNewLocalStorageError(t *testing.T) {
	// 用一个文件路径作为 root，导致 MkdirAll 失败。
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalStorage(file); err == nil {
		t.Fatal("expected error when root is a file")
	}
}

func TestDeleteByPrefix(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, key := range []string{"a/1.txt", "a/2.txt", "a/sub/3.txt", "b/4.txt"} {
		if err := store.Put(ctx, key, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	n, err := store.DeleteByPrefix(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("deleted = %d, want 3", n)
	}
	// 前缀目录连同空的子目录应被清理；其它前缀不受影响。
	if _, err := store.Get(ctx, "a/1.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a/1.txt should be gone, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, "a")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty dir a should be pruned, got %v", err)
	}
	if _, err := store.Get(ctx, "b/4.txt"); err != nil {
		t.Fatalf("b/4.txt should survive: %v", err)
	}

	// 不存在的前缀返回 0。
	if n, err := store.DeleteByPrefix(ctx, "missing"); err != nil || n != 0 {
		t.Fatalf("missing prefix: n=%d err=%v", n, err)
	}
}
