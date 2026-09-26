// Package storage 提供识别服务的存储后端工厂：
// 按 storage.type 配置在本地磁盘（local，默认）与 S3 兼容对象存储（s3）之间切换。
// 本地调试零配置走 local；Docker/K8s 部署通过环境变量切换为 s3（MinIO/OSS/AWS 等）。
package storage

import (
	"fmt"
	"strings"

	"peak/libs/config"
	libsstorage "peak/libs/storage"
)

// New 按配置创建存储后端（进程生命周期内调用一次，返回值作为单例使用）。
//
// storage.type 取值：
//   - "local"（默认，含空值）：本地磁盘，根目录 storage.root（默认 ./data）
//   - "s3"：S3 兼容对象存储，参数见 s3ConfigFrom
//   - 其他值：返回可操作的错误提示
func New(cfg *config.Loader) (libsstorage.FileStorage, error) {
	switch Type(cfg) {
	case "s3":
		sc := s3ConfigFrom(cfg)
		s, err := libsstorage.NewS3Storage(sc)
		if err != nil {
			return nil, fmt.Errorf("init s3 storage (endpoint=%q bucket=%q): %w", sc.Endpoint, sc.Bucket, err)
		}
		return s, nil
	case "local":
		return libsstorage.NewLocalStorage(cfg.String("storage.root", "./data"))
	default:
		return nil, fmt.Errorf("unknown storage.type %q (supported: local, s3)", Type(cfg))
	}
}

// Type 返回规范化后的存储类型（小写、去空白），未配置时为 "local"。
func Type(cfg *config.Loader) string {
	t := strings.ToLower(strings.TrimSpace(cfg.String("storage.type", "local")))
	if t == "" {
		return "local"
	}
	return t
}

// s3ConfigFrom 将 storage.s3.* 配置映射为 S3 存储参数（独立函数便于单测断言）。
func s3ConfigFrom(cfg *config.Loader) libsstorage.Config {
	return libsstorage.Config{
		// 第三方 S3 兼容服务地址（阿里云 OSS / AWS S3 / Ceph 等），需含协议前缀。
		// 为空时由 NewS3Storage 返回明确错误提示。
		Endpoint: cfg.String("storage.s3.endpoint", ""),
		Region:   cfg.String("storage.s3.region", "us-east-1"),
		// 凭证为空时回退 SDK 默认链（环境变量/实例角色，适合 K8s IRSA 等场景）。
		AccessKey: cfg.String("storage.s3.access_key", ""),
		SecretKey: cfg.String("storage.s3.secret_key", ""),
		Bucket:    cfg.String("storage.s3.bucket", "peak"),
		UseSSL:    cfg.Bool("storage.s3.use_ssl", false),
		// AWS S3/OSS 走虚拟主机风格；Ceph/MinIO 等自建服务部署时显式置 true。
		PathStyle: cfg.Bool("storage.s3.path_style", false),
	}
}
