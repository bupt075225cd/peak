package export

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Fetcher 按存储 key 拉取图片字节。接口化便于测试注入假实现。
type Fetcher interface {
	Fetch(ctx context.Context, key string) ([]byte, error)
}

// HTTPFetcher 通过 recognition-service 的文件接口拉取图片。
//
// 图片实际存放在 recognition-service 的 storage 中（本地目录或对象存储）。
// 本服务通过已有的 GET /api/recognition/files/*key 读取，这样无需让两个服务
// 共享 storage 配置，对象存储场景下同样成立。
type HTTPFetcher struct {
	baseURL string
	client  *http.Client
}

// NewHTTPFetcher 创建 HTTP 图片拉取器。baseURL 形如 http://localhost:8082。
func NewHTTPFetcher(baseURL string, timeout time.Duration) *HTTPFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPFetcher{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

// Fetch 拉取指定 key 的图片字节。
func (f *HTTPFetcher) Fetch(ctx context.Context, key string) ([]byte, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("empty image key")
	}
	if f.baseURL == "" {
		return nil, fmt.Errorf("recognition base url is not configured")
	}

	url := f.baseURL + "/api/recognition/files/" + strings.TrimLeft(key, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build image request for %q: %w", key, err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch image %q: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image %q: unexpected status %d", key, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image %q: %w", key, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("fetch image %q: empty response body", key)
	}
	return data, nil
}
