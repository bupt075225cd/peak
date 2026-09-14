package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZhipuProviderDefaults(t *testing.T) {
	z := NewZhipuProvider(ZhipuConfig{})
	if z.Name() != "zhipu" {
		t.Fatalf("expected zhipu, got %s", z.Name())
	}
	if z.dash.model != "glm-5.3-flash" {
		t.Fatalf("expected default glm model, got %s", z.dash.model)
	}
	if !strings.Contains(z.dash.endpoint, "open.bigmodel.cn/api/paas/v4/chat/completions") {
		t.Fatalf("unexpected endpoint: %s", z.dash.endpoint)
	}
	if z.dash.maxTokens != 8192 || z.dash.reasoningEffort != "low" {
		t.Fatalf("unexpected defaults: maxTokens=%d effort=%q", z.dash.maxTokens, z.dash.reasoningEffort)
	}
	// off/none 表示不下发思考强度参数（用于不支持该参数的模型）。
	off := NewZhipuProvider(ZhipuConfig{ReasoningEffort: "off"})
	if off.dash.reasoningEffort != "" {
		t.Fatalf("expected empty reasoning effort for off, got %q", off.dash.reasoningEffort)
	}
}

func TestZhipuEraseHandwritingFallback(t *testing.T) {
	z := NewZhipuProvider(ZhipuConfig{})
	res, err := z.EraseHandwriting(context.Background(), []byte("orig"))
	if err != nil || string(res.ImageData) != "orig" {
		t.Fatalf("expected erasure fallback to original image, got %+v err=%v", res, err)
	}
}

func TestFactoryZhipu(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	content := "recognition:\n  provider: zhipu\n  zhipu:\n    api_key: zk\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoad(t, path)
	p, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if p.Name() != "zhipu" {
		t.Fatalf("expected zhipu, got %s", p.Name())
	}
}

// TestChatClientRequestParams 验证请求体参数按需下发：
// 智谱默认携带 max_tokens 与 reasoning_effort；阿里云默认两者都不下发。
func TestChatClientRequestParams(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	z := NewZhipuProvider(ZhipuConfig{APIKey: "k", Endpoint: srv.URL})
	if _, err := z.RecognizeText(context.Background(), []byte("img")); err != nil {
		t.Fatalf("zhipu chat: %v", err)
	}
	if got["max_tokens"] != float64(8192) || got["reasoning_effort"] != "low" {
		t.Fatalf("unexpected zhipu request params: %v", got)
	}

	a := NewAliyunProvider(AliyunConfig{DashKey: "k", DashEndpoint: srv.URL})
	if _, err := a.RecognizeText(context.Background(), []byte("img")); err != nil {
		t.Fatalf("aliyun chat: %v", err)
	}
	if _, ok := got["max_tokens"]; ok {
		t.Fatalf("dashscope should not send max_tokens: %v", got)
	}
	if _, ok := got["reasoning_effort"]; ok {
		t.Fatalf("dashscope should not send reasoning_effort: %v", got)
	}
}
