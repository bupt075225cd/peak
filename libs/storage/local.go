package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// PresignedURL 本地存储直接返回相对路径作为访问标识。
func (s *LocalStorage) PresignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return key, nil
}
