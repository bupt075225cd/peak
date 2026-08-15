package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// S3Storage 基于 AWS S3 SDK 的对象存储实现。
// 通过 Endpoint 适配不同的 S3 兼容存储：
//   - MinIO：Endpoint 指向 MinIO 地址，PathStyle=true
//   - 阿里云 OSS：Endpoint 指向 OSS 的 S3 兼容端点
//   - AWS S3 / Ceph RGW：Endpoint 指向对应服务
type S3Storage struct {
	client *s3.Client
	bucket string
}

// Config S3 对象存储配置。
type Config struct {
	// Endpoint 服务地址，例如 http://127.0.0.1:9000、https://oss-cn-hangzhou.aliyuncs.com。
	Endpoint string
	// Region 区域，MinIO 通常用 us-east-1；OSS/S3 用实际区域。
	Region string
	// AccessKey / SecretKey 访问凭证。
	AccessKey string
	SecretKey string
	// Bucket 桶名。
	Bucket string
	// UseSSL 是否使用 HTTPS。
	UseSSL bool
	// PathStyle 使用路径风格寻址（MinIO 必须为 true；AWS S3 通常为 false）。
	PathStyle bool
}

// NewS3Storage 创建 S3 兼容对象存储实例。
func NewS3Storage(cfg Config) (*S3Storage, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("storage: endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, errors.New("storage: bucket is required")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	client, err := newS3Client(cfg)
	if err != nil {
		return nil, err
	}
	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

// newS3Client 构建 s3.Client。优先使用显式凭证；无凭证时回退到默认链（环境变量/实例角色）。
func newS3Client(cfg Config) (*s3.Client, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error

	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, err
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = cfg.PathStyle
		},
	}
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	return s3.NewFromConfig(awsCfg, opts...), nil
}

// Put 上传对象。
func (s *S3Storage) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	return err
}

// Get 下载对象，不存在时返回 ErrNotFound。
func (s *S3Storage) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Delete 删除对象。
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// PresignedURL 生成预签名下载 URL。
func (s *S3Storage) PresignedURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	ps := s3.NewPresignClient(s.client)
	req, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(po *s3.PresignOptions) {
		po.Expires = expire
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// isNotFound 判断 S3 错误是否为对象不存在。
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
