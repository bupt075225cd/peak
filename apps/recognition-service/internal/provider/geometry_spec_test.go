package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatTestServer 起一个模拟 OpenAI 兼容接口的 server：固定返回 content，
// 并把请求体解码到 recorded（非 nil 时）供断言使用。
func chatTestServer(t *testing.T, content string, recorded *chatRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected bearer auth")
		}
		if recorded != nil {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, recorded)
		}
		contentBytes, _ := json.Marshal(content)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + string(contentBytes) + `}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// errorChatServer 返回带 error 结构响应的 server。
func errorChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","code":"401"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestExtractGeometrySpecSuccess 验证几何描述提取：模型输出容忍 ```json 包裹，
// 返回提取后的 JSON 原文；题干与修正信息拼入请求。
func TestExtractGeometrySpecSuccess(t *testing.T) {
	spec := `{"title":"图1","canvas":{"width":100,"height":70},"points":[{"name":"A","x":10,"y":10}]}`
	var gotReq chatRequest
	srv := chatTestServer(t, "```json\n"+spec+"\n```", &gotReq)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	got, err := a.ExtractGeometrySpec(context.Background(), []byte("geo-image"), "已知 AB=3", "上次输出的 points 为空")
	if err != nil {
		t.Fatalf("extract geometry spec: %v", err)
	}
	if got != spec {
		t.Fatalf("unexpected spec:\n got: %s\nwant: %s", got, spec)
	}

	var prompt string
	for _, m := range gotReq.Messages {
		if m.Role == "user" && len(m.Content) > 0 {
			prompt = m.Content[0].Text
		}
	}
	if !strings.Contains(prompt, "已知 AB=3") || !strings.Contains(prompt, "上次输出的 points 为空") {
		t.Fatalf("user prompt should contain stem and correction, got %q", prompt)
	}
}

// TestExtractGeometrySpecBadOutput 模型输出不含 JSON 时返回错误。
func TestExtractGeometrySpecBadOutput(t *testing.T) {
	srv := chatTestServer(t, "抱歉，我无法解析这张图", nil)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	if _, err := a.ExtractGeometrySpec(context.Background(), []byte("img"), "", ""); err == nil {
		t.Fatal("expected error for non-JSON output")
	}
}

// TestExtractGeometrySpecChatError 底层对话出错（如鉴权失败）时透传错误。
func TestExtractGeometrySpecChatError(t *testing.T) {
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: errorChatServer(t).URL})

	if _, err := a.ExtractGeometrySpec(context.Background(), []byte("img"), "", ""); err == nil {
		t.Fatal("expected error from chat client")
	}
}
