package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"peak/libs/errors"
	"peak/libs/logger"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

func TestOK(t *testing.T) {
	r := setupRouter()
	r.GET("/", func(c *gin.Context) { OK(c, gin.H{"id": 1}) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":0`) {
		t.Fatalf("expected code 0 in body: %s", body)
	}
}

func TestFailMapping(t *testing.T) {
	cases := []struct {
		code   errors.Code
		status int
	}{
		{errors.CodeInvalidArgument, http.StatusBadRequest},
		{errors.CodeUnauthorized, http.StatusUnauthorized},
		{errors.CodeForbidden, http.StatusForbidden},
		{errors.CodeNotFound, http.StatusNotFound},
		{errors.CodeConflict, http.StatusConflict},
		{errors.CodeInternal, http.StatusInternalServerError},
		{errors.CodeUpstream, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		r := setupRouter()
		r.GET("/", func(c *gin.Context) { Fail(c, errors.New(tc.code, "msg")) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("code %d: expected %d, got %d", tc.code, tc.status, w.Code)
		}
	}
}

func TestFailPlainError(t *testing.T) {
	r := setupRouter()
	r.GET("/", func(c *gin.Context) { Fail(c, errors.New(errors.CodeInternal, "boom")) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHTTPStatus(t *testing.T) {
	if httpStatus(errors.CodeInvalidArgument) != http.StatusBadRequest {
		t.Fatal("bad request mapping")
	}
	if httpStatus(errors.CodeOK) != http.StatusInternalServerError {
		t.Fatal("unknown code should map to 500")
	}
}

func TestTraceIDMiddleware(t *testing.T) {
	r := setupRouter()
	r.Use(TraceID())
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString("trace_id"))
	})

	// 无 traceID 头 -> 自动生成。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Body.String() == "" {
		t.Fatal("expected generated trace id")
	}
	if w.Header().Get("X-Trace-Id") == "" {
		t.Fatal("expected X-Trace-Id header")
	}

	// 传入 X-Trace-Id -> 透传。
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Trace-Id", "custom-trace")
	r.ServeHTTP(w2, req2)
	if w2.Body.String() != "custom-trace" {
		t.Fatalf("expected custom-trace, got %s", w2.Body.String())
	}

	// 传入 X-Request-Id -> 作为 traceID。
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Header.Set("X-Request-Id", "req-123")
	r.ServeHTTP(w3, req3)
	if w3.Body.String() != "req-123" {
		t.Fatalf("expected req-123, got %s", w3.Body.String())
	}
}

func TestRecoverMiddleware(t *testing.T) {
	r := setupRouter()
	r.Use(Recover(logger.NewNop()))
	r.GET("/panic", func(c *gin.Context) { panic("boom") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestAccessLogMiddleware(t *testing.T) {
	r := setupRouter()
	r.Use(TraceID(), AccessLog(logger.NewNop()))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}


