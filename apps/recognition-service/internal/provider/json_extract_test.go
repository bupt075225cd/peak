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

// TestExtractJSONTolerant 覆盖 extractJSON 的容错能力。
func TestExtractJSONTolerant(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"preamble", "好的，结果如下：\n{\"a\":1}", `{"a":1}`},
		{"two objects", "示例 {\"x\":1} 实际输出 {\"a\":2}", `{"x":1}`},
		{"trailing comma", `{"a":1,}`, `{"a":1}`},
		{"trailing comma array", `{"a":[1,2,]}`, `{"a":[1,2]}`},
		{"invalid escape", `{"t":"\angle ABC"}`, `{"t":"\\angle ABC"}`},
		{"truncated", `{"a":[{"b":1}`, `{"a":[{"b":1}]}`},
		{"comma inside string", `{"t":"甲,]"`, `{"t":"甲,]"}`},
	}
	for _, c := range cases {
		got, err := extractJSON(c.in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// 完全不含对象时给出"没有 JSON 对象"的错误，而不是笼统的非法 JSON。
func TestExtractJSONNoObject(t *testing.T) {
	if _, err := extractJSON("抱歉，我无法解析这张图"); err == nil ||
		!strings.Contains(err.Error(), "no JSON object") {
		t.Fatalf("expected no-json-object error, got %v", err)
	}
}

// 几何提取必须下发 response_format=json_object，避免模型夹带解释文字。
func TestExtractGeometrySpecUsesJSONMode(t *testing.T) {
	var gotReq chatRequest
	srv := chatTestServer(t, `{"title":"图1"}`, &gotReq)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	if _, err := a.ExtractGeometrySpec(context.Background(), []byte("img"), "", ""); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if gotReq.ResponseFormat == nil || gotReq.ResponseFormat.Type != "json_object" {
		t.Fatalf("expected response_format=json_object, got %+v", gotReq.ResponseFormat)
	}
}

// 输出被 max_tokens 截断时，要明确报"被截断"，而不是"不是合法 JSON"。
func TestExtractGeometrySpecTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"panels\":[{\"title\""},"finish_reason":"length"}]}`))
	}))
	t.Cleanup(srv.Close)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	_, err := a.ExtractGeometrySpec(context.Background(), []byte("img"), "", "")
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected truncation error, got %v", err)
	}
}

// 非法 JSON 时返回带原始输出的错误，供上层日志记录。
func TestExtractGeometrySpecKeepsRawOutput(t *testing.T) {
	body := `{"choices":[{"message":{"content":"{\"panels\":[}"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	a := NewAliyunProvider(AliyunConfig{DashKey: "test-key", DashEndpoint: srv.URL})

	_, err := a.ExtractGeometrySpec(context.Background(), []byte("img"), "", "")
	moe, ok := err.(*ModelOutputError)
	if !ok {
		t.Fatalf("expected *ModelOutputError, got %T (%v)", err, err)
	}
	if !strings.Contains(moe.Raw, "panels") {
		t.Fatalf("raw output should be preserved, got %q", moe.Raw)
	}
	if moe.Error() != "model output is not valid JSON" {
		t.Fatalf("user-facing message should stay short, got %q", moe.Error())
	}
}

// 请求体应能正确序列化 response_format（防止字段名写错导致静默失效）。
func TestChatRequestMarshalsJSONMode(t *testing.T) {
	b, err := json.Marshal(chatRequest{Model: "m", ResponseFormat: &chatResponseFormat{Type: "json_object"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"response_format":{"type":"json_object"}`) {
		t.Fatalf("unexpected request body: %s", b)
	}
}
