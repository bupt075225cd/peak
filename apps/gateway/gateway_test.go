package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"peak/libs/config"
	"peak/libs/logger"
)

func mustLoad(t *testing.T, content string) *config.Loader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gw.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func setupGateway(t *testing.T, cfgContent string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := mustLoad(t, cfgContent)
	gw := NewGateway(cfg, logger.NewNop())
	r := gin.New()
	gw.RegisterRoutes(r)
	return r
}

func TestHealthz(t *testing.T) {
	r := setupGateway(t, "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestDefaultRoutes(t *testing.T) {
	// 空配置 -> 使用默认路由表。
	cfg := mustLoad(t, "")
	gw := NewGateway(cfg, logger.NewNop())
	if len(gw.routes) == 0 {
		t.Fatal("expected default routes")
	}
	if gw.routes["/api/questions"] == "" {
		t.Fatal("expected questions route")
	}
	if gw.routes["/api/recognition"] == "" {
		t.Fatal("expected recognition route")
	}
	if gw.routes["/api/users"] == "" {
		t.Fatal("expected users route")
	}
}

func TestConfiguredRoutes(t *testing.T) {
	content := `
routes:
  /api/questions: "http://example.com:9000"
  /api/recognition: "http://example.com:9001"
`
	cfg := mustLoad(t, content)
	gw := NewGateway(cfg, logger.NewNop())
	if gw.routes["/api/questions"] != "http://example.com:9000" {
		t.Fatalf("unexpected route: %s", gw.routes["/api/questions"])
	}
	if len(gw.routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(gw.routes))
	}
}

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 普通请求带 CORS 头。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS allow origin")
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected CORS allow methods")
	}

	// OPTIONS 预检请求 -> 204。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodOptions, "/x", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestAuthMiddlewareSetsUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, ""), logger.NewNop())
	r := gin.New()
	// 仅注册 auth 中间件，观察其行为。
	r.Use(gw.authMiddleware())
	r.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString("user_id"))
	})

	// 无 X-User-Id -> mock 用户。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if w.Body.String() != "mock-user-1" {
		t.Fatalf("expected mock user, got %s", w.Body.String())
	}

	// 带 X-User-Id -> 透传。
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.Header.Set("X-User-Id", "user-42")
	r.ServeHTTP(w, req)
	if w.Body.String() != "user-42" {
		t.Fatalf("expected user-42, got %s", w.Body.String())
	}
}

func TestProxyForwardsToBackend(t *testing.T) {
	// 启动一个模拟后端。
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Path", r.URL.Path)
		w.Header().Set("X-Received-Trace", r.Header.Get("X-Trace-Id"))
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	content := "routes:\n  /api/questions: \"" + backend.URL + "\"\n"
	r := setupGateway(t, content)

	req := httptest.NewRequest(http.MethodGet, "/api/questions/1", nil)
	req.Header.Set("X-Trace-Id", "trace-xyz")
	req.Header.Set("X-User-Id", "user-7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from backend, got %d", w.Code)
	}
	// 路径前缀应被剥离。
	if w.Header().Get("X-Backend-Path") != "/1" {
		t.Fatalf("expected stripped path /1, got %s", w.Header().Get("X-Backend-Path"))
	}
	// traceID 应透传。
	if w.Header().Get("X-Received-Trace") != "trace-xyz" {
		t.Fatalf("expected trace trace-xyz, got %s", w.Header().Get("X-Received-Trace"))
	}
}

func TestProxyInvalidBackend(t *testing.T) {
	// 非法 URL 应被跳过（不 panic），但需要保证日志可用。
	content := "routes:\n  /api/bad: \"://bad url\"\n"
	r := setupGateway(t, content)
	w := httptest.NewRecorder()
	// 该前缀不应注册，请求返回 404。
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/bad/x", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid backend, got %d", w.Code)
	}
}
