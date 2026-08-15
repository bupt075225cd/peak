// Package storage 定义统一的文件存储抽象接口，支持本地文件系统与对象存储的无缝切换。
package storage

import (
	"context"
	"time"
)

// FileStorage 文件存储抽象接口，业务层只依赖该接口。
type FileStorage interface {
	// Put 写入对象。
	Put(ctx context.Context, key string, data []byte) error
	// Get 读取对象。
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete 删除对象。
	Delete(ctx context.Context, key string) error
	// PresignedURL 生成带有效期的访问 URL。
	PresignedURL(ctx context.Context, key string, expire time.Duration) (string, error)
}

// ErrNotFound 对象不存在。
var ErrNotFound = &NotFoundError{}

// NotFoundError 对象不存在错误。
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "object not found" }
