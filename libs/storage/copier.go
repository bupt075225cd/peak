package storage

import (
	"context"
	"fmt"
)

// Copier 跨存储对象拷贝器：提交错题时把 recognition-service 存储中的
// 识别产物拷贝到 question-service 自己的存储。
//
// 目标存储实现 ObjectCopier 时（S3Storage）优先走服务端对象拷贝
// （CopyObject，数据不经过应用进程）；否则（LocalStorage）回退为
// 源存储读取 + 目标存储写入。
type Copier struct {
	dst       FileStorage // 目标存储（question-service 自己的桶/目录）
	src       FileStorage // 源存储（recognition-service 的桶/目录），回退路径使用
	srcBucket string      // S3 源桶名；本地后端忽略
}

// NewCopier 创建拷贝器。srcBucket 为源对象所在桶（S3 后端必填，本地后端留空）。
func NewCopier(dst, src FileStorage, srcBucket string) *Copier {
	return &Copier{dst: dst, src: src, srcBucket: srcBucket}
}

// Copy 把 srcKey 对象拷贝到目标存储的 dstKey。
func (c *Copier) Copy(ctx context.Context, srcKey, dstKey string) error {
	if oc, ok := c.dst.(ObjectCopier); ok {
		return oc.CopyFrom(ctx, c.srcBucket, srcKey, dstKey)
	}
	if c.src == nil {
		return fmt.Errorf("copy %q -> %q: no source storage configured", srcKey, dstKey)
	}
	data, err := c.src.Get(ctx, srcKey)
	if err != nil {
		return fmt.Errorf("read source %q: %w", srcKey, err)
	}
	if err := c.dst.Put(ctx, dstKey, data); err != nil {
		return fmt.Errorf("write destination %q: %w", dstKey, err)
	}
	return nil
}
