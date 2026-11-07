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

	"github.com/prometheus/client_golang/prometheus/testutil"

	"peak/libs/observability"
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
	if z.dash.maxTokens != 16384 || z.dash.reasoningEffort != "low" {
		t.Fatalf("unexpected defaults: maxTokens=%d effort=%q", z.dash.maxTokens, z.dash.reasoningEffort)
	}
	// 几何描述提取默认低温度采样：坐标直出的随机漂移随温度明显加剧。
	if z.geometryTemperature != 0.1 {
		t.Fatalf("expected default geometry temperature 0.1, got %v", z.geometryTemperature)
	}
	// 负数表示不下发 temperature 参数。
	noTemp := NewZhipuProvider(ZhipuConfig{GeometryTemperature: -1})
	if noTemp.geometryTemperature != 0 {
		t.Fatalf("expected temperature off for negative config, got %v", noTemp.geometryTemperature)
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
	if got["max_tokens"] != float64(16384) || got["reasoning_effort"] != "low" {
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

// TestGeometrySpecSamplingParams 验证几何描述提取走专用采样参数：
// 低温度（默认 0.1）+ 几何专用思考强度；其余调用（RecognizeText）不下发 temperature。
func TestGeometrySpecSamplingParams(t *testing.T) {
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"panels\":[]}"}}]}`))
	}))
	defer srv.Close()

	z := NewZhipuProvider(ZhipuConfig{APIKey: "k", Endpoint: srv.URL})
	if _, err := z.ExtractGeometrySpec(context.Background(), []byte("img"), "题干", ""); err != nil {
		t.Fatalf("extract geometry spec: %v", err)
	}
	if got["temperature"] != float64(0.1) {
		t.Fatalf("geometry spec should send low temperature, got %v", got["temperature"])
	}
	if got["reasoning_effort"] != "high" {
		t.Fatalf("geometry spec should send high effort, got %v", got["reasoning_effort"])
	}

	// 普通文本调用不下发 temperature（使用服务端默认）。
	if _, err := z.RecognizeText(context.Background(), []byte("img")); err != nil {
		t.Fatalf("recognize text: %v", err)
	}
	if _, ok := got["temperature"]; ok {
		t.Fatalf("regular chat should not send temperature: %v", got["temperature"])
	}
}

// TestChatUsageMetrics 响应中的 usage 被解析并计入 token 指标（按 operation 分维度）。
func TestChatUsageMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],` +
			`"usage":{"prompt_tokens":120,"completion_tokens":45,"total_tokens":165}}`))
	}))
	defer srv.Close()

	z := NewZhipuProvider(ZhipuConfig{APIKey: "k", Endpoint: srv.URL})
	ctx := observability.WithAIOperation(context.Background(), "geometry_spec_extract")
	if _, err := z.RecognizeText(ctx, []byte("img")); err != nil {
		t.Fatalf("recognize text: %v", err)
	}
	if got := testutil.ToFloat64(observability.AITokensTotal.WithLabelValues("zhipu", "geometry_spec_extract", "prompt")); got != 120 {
		t.Fatalf("prompt tokens metric: got %v", got)
	}
	if got := testutil.ToFloat64(observability.AITokensTotal.WithLabelValues("zhipu", "geometry_spec_extract", "completion")); got != 45 {
		t.Fatalf("completion tokens metric: got %v", got)
	}

	// 截断响应（finish_reason=length）同样携带 usage，也应计入后再报错。
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"length"}],` +
			`"usage":{"prompt_tokens":10,"completion_tokens":8192,"total_tokens":8202}}`))
	}))
	defer srv2.Close()

	z2 := NewZhipuProvider(ZhipuConfig{APIKey: "k", Endpoint: srv2.URL})
	if _, err := z2.RecognizeText(ctx, []byte("img")); err == nil {
		t.Fatal("expected truncation error")
	}
	if got := testutil.ToFloat64(observability.AITokensTotal.WithLabelValues("zhipu", "geometry_spec_extract", "completion")); got != 45+8192 {
		t.Fatalf("truncated usage should still be recorded, got %v", got)
	}
}
