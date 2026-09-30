package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"peak/libs/logger"
)

func setupMonitorRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerMonitorReport(r, logger.NewNop().Logger)
	return r
}

func TestMonitorReportAcceptsValidEvents(t *testing.T) {
	r := setupMonitorRouter()
	body := `{"events":[
		{"type":"error","name":"js_error","message":"x","path":"/list","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"},
		{"type":"webvital","name":"LCP","path":"/home","data":{"value":1200,"rating":"good"}},
		{"type":"pv","name":"pv","path":"/home","data":{"from":"(direct)"}}
	]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/report", strings.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestMonitorReportRejectsBadInput(t *testing.T) {
	r := setupMonitorRouter()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"invalid json", `{not json`, http.StatusNoContent},
		{"oversize body", `{"events":"` + strings.Repeat("x", 70<<10) + `"}`, http.StatusRequestEntityTooLarge},
		{"unknown type dropped", `{"events":[{"type":"evil","name":"n"}]}`, http.StatusNoContent},
		{"empty name dropped", `{"events":[{"type":"pv","name":""}]}`, http.StatusNoContent},
		{"oversize name dropped", `{"events":[{"type":"pv","name":"` + strings.Repeat("n", 100) + `"}]}`, http.StatusNoContent},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/monitor/report", strings.NewReader(tc.body)))
		if w.Code != tc.want {
			t.Fatalf("%s: expected %d, got %d", tc.name, tc.want, w.Code)
		}
	}
}

func TestSanitizeTraceID(t *testing.T) {
	cases := map[string]string{
		"4bf92f3577b34da6a3ce929d0e0e4736": "4bf92f3577b34da6a3ce929d0e0e4736",
		"":                                 "",
		"with space injected":              "",            // 非法字符 -> 清空
		"<script>alert(1)</script>":        "",            // 防日志注入
		strings.Repeat("a", 100):           strings.Repeat("a", 64), // 超长截断
	}
	for in, want := range cases {
		if got := sanitizeTraceID(in); got != want {
			t.Fatalf("sanitizeTraceID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMonitorReportIsPublic(t *testing.T) {
	if !isPublicPath("/api/monitor/report") {
		t.Fatal("monitor report should be public")
	}
}
