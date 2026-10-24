package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	tok, err := auth.Issue(userID, "", testJWTSecret, time.Hour)
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

// 安全头必须在鉴权失败时也存在：401/429 响应同样可能渲染到浏览器，
// 缺少 CSP 与 nosniff 会让防护在最需要的时候失效。
func TestSecurityHeadersOnAllResponses(t *testing.T) {
	r := setupGateway(t, "")

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
		{"unauthorized ledger", http.MethodGet, "/api/ledger/meta", http.StatusUnauthorized},
		// 未知路径同样先被鉴权拦下，不泄露路由存在性。
		{"unknown route", http.MethodGet, "/api/unknown/path", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-Client-Id", "app-peak")
			r.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			for header, want := range map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "no-referrer",
			} {
				if got := w.Header().Get(header); got != want {
					t.Fatalf("%s = %q, want %q", header, got, want)
				}
			}
			if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
				t.Fatalf("CSP = %q", csp)
			}
		})
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
	gw := NewGateway(mustLoad(t, ""), logger.NewNop())
	r := gin.New()
	r.Use(gw.corsMiddleware())
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
	r.GET("/api/recognition/files/transient/geometry/task_1_1.svg", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/mistakes/files/committed/1.png", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/metrics", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 白名单路径无需令牌。
	for _, path := range []string{
		"/api/users/auth/sms/code",
		"/api/recognition/files/transient/geometry/task_1_1.svg",
		"/api/mistakes/files/committed/1.png",
		"/healthz",
		"/metrics",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("public path %s: expected 200, got %d", path, w.Code)
		}
	}

	// 非白名单的识别接口仍需鉴权。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/recognition/tasks/1", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for protected path, got %d", w.Code)
	}
}

func TestAuthMiddlewarePoetryPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, "auth:\n  jwt_secret: \""+testJWTSecret+"\"\n"), logger.NewNop())
	r := gin.New()
	r.Use(gw.authMiddleware())
	r.GET("/api/poetry/pieces", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/poetry/pieces/3", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/poetry/pieces/3/related", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/poetry/stats/dynasties", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/poetry/chat", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 诗歌只读 GET 接口无需令牌（浏览免登录）。
	for _, path := range []string{
		"/api/poetry/pieces",
		"/api/poetry/pieces/3",
		"/api/poetry/pieces/3/related",
		"/api/poetry/stats/dynasties",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("public poetry GET %s: expected 200, got %d", path, w.Code)
		}
	}

	// chat 接口不在白名单，未登录仍需 401。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/poetry/chat", strings.NewReader(`{"question":"q"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for poetry chat without token, got %d", w.Code)
	}

	// 携带有效令牌的 chat 请求放行。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodPost, "/api/poetry/chat", issueTestToken(t, 1)))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for poetry chat with token, got %d", w.Code)
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

func TestInjectTraceContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 32 位 hex trace_id（无激活 span）：应构造不采样的 traceparent + 透传 X-Trace-Id。
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("trace_id", "4bf92f3577b34da6a3ce929d0e0e4736")
		c.Next()
	})
	r.GET("/", func(c *gin.Context) {
		injectTraceContext(c)
		c.String(http.StatusOK, c.Request.Header.Get("traceparent")+"|"+c.Request.Header.Get("X-Trace-Id"))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.HasPrefix(w.Body.String(), "00-4bf92f3577b34da6a3ce929d0e0e4736-") {
		t.Fatalf("expected traceparent with hex trace id, got %s", w.Body.String())
	}
	if !strings.HasSuffix(w.Body.String(), "|4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Fatalf("expected X-Trace-Id passthrough, got %s", w.Body.String())
	}

	// 非 hex trace_id：仅透传 X-Trace-Id，不写 traceparent。
	r2 := gin.New()
	r2.Use(func(c *gin.Context) {
		c.Set("trace_id", "trace-xyz")
		c.Next()
	})
	r2.GET("/", func(c *gin.Context) {
		injectTraceContext(c)
		c.String(http.StatusOK, c.Request.Header.Get("traceparent")+"|"+c.Request.Header.Get("X-Trace-Id"))
	})
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/", nil))
	if w2.Body.String() != "|trace-xyz" {
		t.Fatalf("expected only X-Trace-Id passthrough, got %s", w2.Body.String())
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

// ── 多应用隔离（JWT aud）与 CORS 白名单 ──────────────────────────

// audTestConfig 构造带受众映射的网关配置。
func audTestConfig(extra string) string {
	return `auth:
  jwt_secret: "` + testJWTSecret + `"
  audience_routes:
    /api/mistakes: ["app-mistake"]
    /api/poetry: ["app-poetry"]
` + extra
}

func setupAudGateway(t *testing.T, extra string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, audTestConfig(extra)), logger.NewNop())
	r := gin.New()
	r.Use(gw.authMiddleware())
	r.GET("/api/mistakes", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/poetry/chat", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestAudienceMismatchRejected(t *testing.T) {
	r := setupAudGateway(t, "")

	// 错题本应用的令牌访问诗歌接口 → 403（应用间令牌不通用）。
	tokenPoetry, _ := auth.Issue(1, "app-poetry", testJWTSecret, time.Hour)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/api/mistakes", tokenPoetry))
	if w.Code != http.StatusForbidden {
		t.Fatalf("audience mismatch: expected 403, got %d", w.Code)
	}

	// 匹配的受众 → 200。
	tokenMistake, _ := auth.Issue(1, "app-mistake", testJWTSecret, time.Hour)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/api/mistakes", tokenMistake))
	if w.Code != http.StatusOK {
		t.Fatalf("audience match: expected 200, got %d", w.Code)
	}
}

func TestLegacyTokenAllowedByDefault(t *testing.T) {
	r := setupAudGateway(t, "")

	// 无 aud 的历史令牌：默认兼容放行。
	legacy, _ := auth.Issue(1, "", testJWTSecret, time.Hour)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, requestWithToken(http.MethodGet, "/api/mistakes", legacy))
	if w.Code != http.StatusOK {
		t.Fatalf("legacy token: expected 200, got %d", w.Code)
	}

	// 关闭兼容开关 → 401。
	r2 := setupAudGateway(t, "  allow_legacy_tokens: false\n")
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, requestWithToken(http.MethodGet, "/api/mistakes", legacy))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("legacy token strict: expected 401, got %d", w2.Code)
	}
}

func TestCORSOriginWhitelist(t *testing.T) {
	content := `auth:
  jwt_secret: "` + testJWTSecret + `"
cors:
  allowed_origins: ["https://mistake.example.com", "https://poetry.example.com"]
`
	gin.SetMode(gin.TestMode)
	gw := NewGateway(mustLoad(t, content), logger.NewNop())
	r := gin.New()
	r.Use(gw.corsMiddleware())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 白名单内来源：回显具体 Origin 并携带 Vary: Origin。
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://mistake.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://mistake.example.com" {
		t.Fatalf("allow origin = %q, want echoed origin", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("expected Vary: Origin")
	}

	// 白名单外来源：不返回放行头。
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("non-whitelisted origin should not be allowed, got %q", got)
	}

	// 未配置白名单：回退 *（本地开发态）。
	gw2 := NewGateway(mustLoad(t, ""), logger.NewNop())
	r2 := gin.New()
	r2.Use(gw2.corsMiddleware())
	r2.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/x", nil))
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("empty whitelist should fall back to *, got %q", got)
	}
}
