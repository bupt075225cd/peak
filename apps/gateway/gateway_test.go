package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"peak/libs/auth"
	"peak/libs/config"
	"peak/libs/logger"
)

const testJWTSecret = "gw-test-secret"

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
	if cfgContent == "" {
		cfgContent = "auth:\n  jwt_secret: \"" + testJWTSecret + "\"\n"
	}
	gin.SetMode(gin.TestMode)
	cfg := mustLoad(t, cfgContent)
	gw := NewGateway(cfg, logger.NewNop())
	r := gin.New()
	gw.RegisterRoutes(r)
	return r
}

// issueTestToken 签发测试用 JWT。
func issueTestToken(t *testing.T, userID uint64) string {
	t.Helper()
	tok, err := auth.Issue(userID, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// requestWithToken 构造带 Bearer 令牌的请求。
func requestWithToken(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
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
	if gw.routes["/api/mistakes"] == "" {
		t.Fatal("expected mistakes route")
	}
	if gw.routes["/api/categories"] == "" {
		t.Fatal("expected categories route")
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

func TestAuthMiddlewareJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, "auth:\n  jwt_secret: \""+testJWTSecret+"\"\n"), logger.NewNop())
	r := gin.New()
	r.Use(gw.authMiddleware())
	r.GET("/whoami", func(c *gin.Context) {
		c.String(http.StatusOK, "%d", c.MustGet("user_id"))
	})

	// 有效令牌 -> 解析出真实用户 ID。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/whoami", issueTestToken(t, 42)))
	if w.Body.String() != "42" {
		t.Fatalf("expected uid 42, got %s", w.Body.String())
	}

	// 缺失令牌 -> 401。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/whoami", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	// 伪造令牌 -> 401。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/whoami", "forged.token.here"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with forged token, got %d", w.Code)
	}

	// 伪造的 X-User-Id 头应被覆盖为真实身份（而非透传）。
	w = httptest.NewRecorder()
	req := requestWithToken(http.MethodGet, "/whoami", issueTestToken(t, 7))
	req.Header.Set("X-User-Id", "999999")
	r.ServeHTTP(w, req)
	if w.Body.String() != "7" {
		t.Fatalf("forged X-User-Id should be overridden, got %s", w.Body.String())
	}
}

func TestAuthMiddlewarePublicPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, "auth:\n  jwt_secret: \""+testJWTSecret+"\"\n"), logger.NewNop())
	r := gin.New()
	r.Use(gw.authMiddleware())
	r.GET("/api/users/auth/sms/code", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/metrics", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 白名单路径无需令牌。
	for _, path := range []string{"/api/users/auth/sms/code", "/healthz", "/metrics"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("public path %s: expected 200, got %d", path, w.Code)
		}
	}
}

func TestAuthMiddlewareInjectsRealIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, "auth:\n  jwt_secret: \""+testJWTSecret+"\"\n"), logger.NewNop())
	r := gin.New()
	_ = r.SetTrustedProxies(nil) // 与 RegisterRoutes 中的生产配置一致
	r.Use(gw.authMiddleware())
	r.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.Request.Header.Get("X-Real-IP")) })

	// 外部伪造的 X-Real-IP 应被网关侧 ClientIP 覆盖。
	w := httptest.NewRecorder()
	req := requestWithToken(http.MethodGet, "/ip", issueTestToken(t, 1))
	req.Header.Set("X-Real-IP", "6.6.6.6")
	req.RemoteAddr = "192.168.1.50:12345"
	r.ServeHTTP(w, req)
	if w.Body.String() != "192.168.1.50" {
		t.Fatalf("X-Real-IP = %s, want gateway ClientIP", w.Body.String())
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

	content := "auth:\n  jwt_secret: \"" + testJWTSecret + "\"\nroutes:\n  /api/questions: \"" + backend.URL + "\"\n"
	r := setupGateway(t, content)

	req := requestWithToken(http.MethodGet, "/api/questions/1", issueTestToken(t, 7))
	req.Header.Set("X-Trace-Id", "trace-xyz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from backend, got %d", w.Code)
	}
	// 路径应完整透传（不剥离前缀），与后端注册的完整路径一致。
	if w.Header().Get("X-Backend-Path") != "/api/questions/1" {
		t.Fatalf("expected path /api/questions/1, got %s", w.Header().Get("X-Backend-Path"))
	}
	// traceID 应透传。
	if w.Header().Get("X-Received-Trace") != "trace-xyz" {
		t.Fatalf("expected trace trace-xyz, got %s", w.Header().Get("X-Received-Trace"))
	}
}

func TestProxyForwardsPrefixRoot(t *testing.T) {
	// 前缀本身（无尾随斜杠）也应被转发，例如 GET /api/questions 列表请求。
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	content := "auth:\n  jwt_secret: \"" + testJWTSecret + "\"\nroutes:\n  /api/questions: \"" + backend.URL + "\"\n"
	r := setupGateway(t, content)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/api/questions", issueTestToken(t, 7)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from backend, got %d", w.Code)
	}
	if w.Header().Get("X-Backend-Path") != "/api/questions" {
		t.Fatalf("expected path /api/questions, got %s", w.Header().Get("X-Backend-Path"))
	}
}

func TestProxyInvalidBackend(t *testing.T) {
	// 非法 URL 应被跳过（不 panic），但需要保证日志可用。
	content := "auth:\n  jwt_secret: \"" + testJWTSecret + "\"\nroutes:\n  /api/bad: \"://bad url\"\n"
	r := setupGateway(t, content)
	w := httptest.NewRecorder()
	// 该前缀不应注册，携带有效令牌的请求返回 404。
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/api/bad/x", issueTestToken(t, 1)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid backend, got %d", w.Code)
	}
}
