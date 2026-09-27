package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage 本地文件系统实现，按 key 组织目录。
type LocalStorage struct {
	root string
}

// NewLocalStorage 创建本地存储实例。
func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) path(key string) string {
	return filepath.Join(s.root, key)
}

// Put 写入对象，自动创建父目录。
func (s *LocalStorage) Put(_ context.Context, key string, data []byte) error {
	p := s.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Get 读取对象，不存在时返回 ErrNotFound。
func (s *LocalStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, err := os.ReadFile(s.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return data, err
}

// Delete 删除对象。
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// DeleteByPrefix 删除 key 以 prefix 开头的全部对象，返回删除数量。
// 与 S3 前缀语义一致：prefix 既可能是目录（a/），也可能是文件名前缀
// （geometry/task_1. 匹配 task_1.svg 而不影响 task_10.svg）。
// 删除后尽力清理因此变空的目录。
func (s *LocalStorage) DeleteByPrefix(_ context.Context, prefix string) (int, error) {
	if prefix == "" {
		return 0, nil
	}
	dir := s.path(prefix)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		// 前缀恰好精确命中一个文件。
		if err := os.Remove(dir); err != nil {
			return 0, err
		}
		return 1, nil
	}

	// withinRoot 判断 path 是否位于存储根目录内（含等于 root 本身）。
	withinRoot := func(p string) bool {
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return false
		}
		return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}

	// 找到 prefix 路径下实际存在的最深祖先目录，从那里遍历。
	// 上爬不得越出存储根目录：prefix 目录不存在时按无匹配处理，
	// 否则会爬到 root 之外（如 /tmp），误删无关的空目录与文件。
	walkDir := dir
	for {
		if _, err := os.Stat(walkDir); err == nil {
			break
		}
		parent := filepath.Dir(walkDir)
		if parent == walkDir || !withinRoot(parent) {
			return 0, nil // prefix 目录不存在，视为无匹配。
		}
		walkDir = parent
	}

	count := 0
	prunable := []string{}
	err := filepath.WalkDir(walkDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != walkDir {
				prunable = append(prunable, path)
			}
			return nil
		}
		key := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(path, s.root), string(os.PathSeparator)))
		if strings.HasPrefix(key, prefix) {
			if rmErr := os.Remove(path); rmErr == nil {
				count++
			}
		}
		return nil
	})
	if err != nil {
		return count, err
	}
	// 删除变空的子目录（按路径长度倒序，先删最深的）。
	for i := len(prunable) - 1; i >= 0; i-- {
		_ = os.Remove(prunable[i])
	}
	_ = os.Remove(walkDir)
	return count, nil
}

// PresignedURL 本地存储直接返回相对路径作为访问标识。
func (s *LocalStorage) PresignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return key, nil
}
