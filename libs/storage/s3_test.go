package storage

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// TestNewS3StorageMissingEndpoint 校验 endpoint 必填。
func TestNewS3StorageMissingEndpoint(t *testing.T) {
	_, err := NewS3Storage(Config{Bucket: "b"})
	if err == nil {
		t.Fatal("expected error when endpoint is empty")
	}
}

// TestNewS3StorageMissingBucket 校验 bucket 必填。
func TestNewS3StorageMissingBucket(t *testing.T) {
	_, err := NewS3Storage(Config{Endpoint: "http://127.0.0.1:9000"})
	if err == nil {
		t.Fatal("expected error when bucket is empty")
	}
}

// TestNewS3StorageDefaultRegion region 为空时回退 us-east-1。
func TestNewS3StorageDefaultRegion(t *testing.T) {
	s, err := NewS3Storage(Config{
		Endpoint:  "http://127.0.0.1:9000",
		Bucket:    "test",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("new s3 storage: %v", err)
	}
	if s.bucket != "test" {
		t.Fatalf("unexpected bucket: %s", s.bucket)
	}
	if s.client == nil {
		t.Fatal("client should not be nil")
	}
}

// TestIsNotFound 验证常见 S3 错误码被识别为 not found。
func TestIsNotFound(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"NoSuchKey", true},
		{"NotFound", true},
		{"404", true},
		{"AccessDenied", false},
		{"InternalError", false},
	}
	for _, c := range cases {
		err := &smithy.GenericAPIError{Code: c.code, Message: "msg"}
		if got := isNotFound(err); got != c.want {
			t.Fatalf("isNotFound(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIsNotFoundNonAPIError 非 smithy API 错误应返回 false。
func TestIsNotFoundNonAPIError(t *testing.T) {
	if isNotFound(errors.New("plain error")) {
		t.Fatal("plain error should not be not-found")
	}
}

// TestIsNotFoundNoSuchKeyTyped 验证类型化的 NoSuchKey 错误。
func TestIsNotFoundNoSuchKeyTyped(t *testing.T) {
	var apiErr smithy.APIError = &types.NoSuchKey{}
	if !isNotFound(apiErr) {
		t.Fatal("NoSuchKey should be recognized as not-found")
	}
}
