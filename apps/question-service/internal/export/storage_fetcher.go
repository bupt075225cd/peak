package export

import (
	"context"
	"fmt"
	"strings"

	"peak/libs/storage"
)

// StorageFetcher 直读对象存储的图片拉取器：替代原 HTTPFetcher，
// 不再经 recognition-service 转发。
//
// key 优先按 committed/ 正式区解析；未命中时回退原 key（兼容未迁移的
// 存量数据，其产物仍在旧前缀 original/、geometry/ 下）。
type StorageFetcher struct {
	store storage.FileStorage
}

// NewStorageFetcher 创建存储图片拉取器。
func NewStorageFetcher(store storage.FileStorage) *StorageFetcher {
	return &StorageFetcher{store: store}
}

// Fetch 拉取指定 key 的图片字节。
func (f *StorageFetcher) Fetch(ctx context.Context, key string) ([]byte, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("empty image key")
	}
	if !strings.HasPrefix(key, "committed/") {
		key = "committed/" + key
	}
	data, err := f.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetch image %q: %w", key, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("fetch image %q: empty object", key)
	}
	return data, nil
}
