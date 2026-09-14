package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSidecarRedrawSuccess 验证 sidecar 客户端解析 /redraw 响应。
func TestSidecarRedrawSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redraw" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"svgs": [
				{"title": "图1", "svg": "<svg xmlns='http://www.w3.org/2000/svg'>fig1</svg>"},
				{"title": "图2", "svg": "<svg xmlns='http://www.w3.org/2000/svg'>fig2</svg>"}
			],
			"report": {"max_hard": 1e-6, "max_soft": 0.01, "max_all": 0.01,
				"per_constraint": [{"index": 0, "type": "horizontal", "severity": "hard", "residual": 1e-6}]}
		}`))
	}))
	defer srv.Close()

	c := NewSidecarRedraw(srv.URL, 0)
	res, err := c.RedrawGeometry(context.Background(), `{"points":["A","B"]}`)
	if err != nil {
		t.Fatalf("redraw: %v", err)
	}
	if len(res.SVGs) != 2 {
		t.Fatalf("expected 2 svgs, got %d", len(res.SVGs))
	}
	if !strings.HasPrefix(string(res.SVGs[0].Data), "<svg") {
		t.Fatalf("unexpected svg: %q", res.SVGs[0].Data)
	}
	if res.SVGs[0].Title != "图1" || res.SVGs[1].Title != "图2" {
		t.Fatalf("unexpected svg titles: %+v", res.SVGs)
	}
	if res.Report == nil || res.Report.MaxHard != 1e-6 {
		t.Fatalf("unexpected report: %+v", res.Report)
	}
	if len(res.Report.PerConstraint) != 1 || res.Report.PerConstraint[0].Type != "horizontal" {
		t.Fatalf("unexpected per_constraint: %+v", res.Report.PerConstraint)
	}
}

// TestSidecarRedrawError 验证 sidecar 返回 400 时透传 detail。
func TestSidecarRedrawError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail": "未知约束类型: bad"}`))
	}))
	defer srv.Close()

	c := NewSidecarRedraw(srv.URL, 0)
	_, err := c.RedrawGeometry(context.Background(), `{}`)
	if err == nil || !strings.Contains(err.Error(), "未知约束类型") {
		t.Fatalf("expected detail passthrough, got: %v", err)
	}
}

// TestExtractJSON 验证模型输出的 JSON 提取容错。
func TestExtractJSON(t *testing.T) {
	cases := []struct {
		in      string
		wantSub string
	}{
		{`{"a":1}`, `"a":1`},
		{"```json\n{\"a\": 2}\n```", `"a": 2`},
		{"好的，以下是解析结果：\n{\"points\":[\"A\"]} 请查收", `"points":["A"]`},
	}
	for _, c := range cases {
		got, err := extractJSON(c.in)
		if err != nil {
			t.Fatalf("extractJSON(%q): %v", c.in, err)
		}
		if !strings.Contains(got, c.wantSub) {
			t.Fatalf("extractJSON(%q) = %q, want contains %q", c.in, got, c.wantSub)
		}
	}
	if _, err := extractJSON("no json here"); err == nil {
		t.Fatal("expected error for non-JSON output")
	}
}

// TestMockProviderImplementsRedrawInterfaces 验证 mock provider 具备几何描述提取能力，
// 且返回的 spec 为坐标直出 schema（含 canvas/points）。
func TestMockProviderImplementsRedrawInterfaces(t *testing.T) {
	var m Provider = NewMockProvider()
	if _, ok := m.(GeometrySpecExtractor); !ok {
		t.Fatal("MockProvider should implement GeometrySpecExtractor")
	}
	spec, err := NewMockProvider().ExtractGeometrySpec(context.Background(), nil, "", "")
	if err != nil || !strings.Contains(spec, `"canvas"`) || !strings.Contains(spec, `"points"`) {
		t.Fatalf("unexpected mock spec: %q, err=%v", spec, err)
	}
}
