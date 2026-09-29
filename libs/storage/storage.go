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
	// DeleteByPrefix 删除指定前缀下的全部对象，返回删除数量。
	// 用于按任务清理中间产物（如 geometry/task_<id>*），无匹配对象时返回 0。
	DeleteByPrefix(ctx context.Context, prefix string) (int, error)
	// PresignedURL 生成带有效期的访问 URL。
	PresignedURL(ctx context.Context, key string, expire time.Duration) (string, error)
}

// ObjectCopier 支持跨桶/跨区域对象拷贝的可选扩展接口。
// S3Storage 通过服务端 CopyObject 实现（数据不经过应用进程）；
// LocalStorage 不实现该接口，由调用方回退为源读+目标写。
type ObjectCopier interface {
	// CopyFrom 把 srcBucket/srcKey 处的对象拷贝到本存储的 dstKey。
	// 源桶与本存储必须可由同一 Endpoint/凭证访问。
	CopyFrom(ctx context.Context, srcBucket, srcKey, dstKey string) error
}

// ErrNotFound 对象不存在。
var ErrNotFound = &NotFoundError{}

// NotFoundError 对象不存在错误。
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "object not found" }
