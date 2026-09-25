package storage

import (
	"os"
	"path/filepath"
	"testing"

	"peak/libs/config"
	libsstorage "peak/libs/storage"
)

// loadConfig 在临时目录写入 YAML 并加载，避免环境变量占位干扰断言。
func loadConfig(t *testing.T, content string) *config.Loader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func TestNewDefaultLocalStorage(t *testing.T) {
	// 空 storage 段：默认 local，零配置可跑。
	cfg := loadConfig(t, "server:\n  port: \"8082\"\n")
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := s.(*libsstorage.LocalStorage); !ok {
		t.Fatalf("expected *LocalStorage, got %T", s)
	}
	if got := Type(cfg); got != "local" {
		t.Fatalf("Type() = %q, want local", got)
	}
}

func TestNewExplicitLocal(t *testing.T) {
	cfg := loadConfig(t, "storage:\n  type: local\n  root: ./testdata\n")
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := s.(*libsstorage.LocalStorage); !ok {
		t.Fatalf("expected *LocalStorage, got %T", s)
	}
}

func TestNewS3(t *testing.T) {
	cfg := loadConfig(t, `storage:
  type: s3
  s3:
    endpoint: "http://minio:9000"
    region: "us-east-1"
    access_key: "ak"
    secret_key: "sk"
    bucket: "peak"
    use_ssl: "false"
    path_style: "true"
`)
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := s.(*libsstorage.S3Storage); !ok {
		t.Fatalf("expected *S3Storage, got %T", s)
	}
}

func TestS3ConfigFrom(t *testing.T) {
	cfg := loadConfig(t, `storage:
  type: s3
  s3:
    endpoint: "https://oss-cn-hangzhou.aliyuncs.com"
    region: "cn-hangzhou"
    access_key: "my-ak"
    secret_key: "my-sk"
    bucket: "my-bucket"
    use_ssl: "true"
    path_style: "false"
`)
	got := s3ConfigFrom(cfg)
	want := libsstorage.Config{
		Endpoint:  "https://oss-cn-hangzhou.aliyuncs.com",
		Region:    "cn-hangzhou",
		AccessKey: "my-ak",
		SecretKey: "my-sk",
		Bucket:    "my-bucket",
		UseSSL:    true,
		PathStyle: false,
	}
	if got != want {
		t.Fatalf("s3ConfigFrom() = %+v, want %+v", got, want)
	}
}

func TestS3ConfigFromDefaults(t *testing.T) {
	// 未配置 s3 子段时的兜底默认值（适配 MinIO）。
	cfg := loadConfig(t, "storage:\n  type: s3\n")
	got := s3ConfigFrom(cfg)
	if got.Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("Endpoint = %q", got.Endpoint)
	}
	if got.Bucket != "peak" {
		t.Fatalf("Bucket = %q", got.Bucket)
	}
	if !got.PathStyle {
		t.Fatal("PathStyle default should be true (MinIO)")
	}
	if got.UseSSL {
		t.Fatal("UseSSL default should be false")
	}
}

func TestNewS3MissingBucket(t *testing.T) {
	cfg := loadConfig(t, "storage:\n  type: s3\n  s3:\n    bucket: \"\"\n")
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestNewUnknownType(t *testing.T) {
	cfg := loadConfig(t, "storage:\n  type: oss-native\n")
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for unknown storage.type")
	}
}
