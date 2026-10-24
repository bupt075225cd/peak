package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"peak/libs/logger"
)

func TestRateLimiterTokenBucket(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rl := newRateLimiter(1, 2, []string{"/api/ledger"})
	rl.now = func() time.Time { return now }

	first := rl.allow(1)
	second := rl.allow(1)
	if !first || !second {
		t.Fatal("first two requests should pass")
	}
	if rl.allow(1) {
		t.Fatal("third request should be limited")
	}
	// 其他用户独立计数。
	if !rl.allow(2) {
		t.Fatal("other user should have its own bucket")
	}
	// 经过 1 秒补充 1 个令牌。
	now = now.Add(time.Second)
	if !rl.allow(1) {
		t.Fatal("bucket should refill after rps interval")
	}
}

func TestRateLimiterMatch(t *testing.T) {
	rl := newRateLimiter(1, 1, []string{"/api/ledger", ""})
	if !rl.match("/api/ledger/items") {
		t.Fatal("ledger path should match")
	}
	if rl.match("/api/poetry/pieces") {
		t.Fatal("poetry path should not match")
	}
}

func TestRateLimiterGC(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rl := newRateLimiter(1, 1, []string{"/api/ledger"})
	rl.now = func() time.Time { return now }
	rl.allow(9)
	if len(rl.buckets) != 1 {
		t.Fatalf("buckets = %d, want 1", len(rl.buckets))
	}
	// 超过 30 分钟未活跃后应被清理。
	now = now.Add(31 * time.Minute)
	rl.allow(10)
	if _, ok := rl.buckets[9]; ok {
		t.Fatal("stale bucket should be garbage collected")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Unix(1_700_000_000, 0)
	rl := newRateLimiter(1, 1, []string{"/api/ledger"})
	rl.now = func() time.Time { return now }

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if uid := c.GetHeader("X-User-Id"); uid != "" {
			c.Set("user_id", uint64(7))
		}
		c.Next()
	})
	r.Use(rl.middleware())
	r.GET("/api/ledger/items", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/poetry/pieces", func(c *gin.Context) { c.Status(http.StatusOK) })

	send := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-User-Id", "7")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := send("/api/ledger/items"); w.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", w.Code)
	}
	w := send("/api/ledger/items")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
	// 未配置前缀的路径不受限流影响。
	if w = send("/api/poetry/pieces"); w.Code != http.StatusOK {
		t.Fatalf("poetry request = %d, want 200", w.Code)
	}
	// 无身份的请求直接放行（鉴权中间件已负责拒绝）。
	req := httptest.NewRequest(http.MethodGet, "/api/ledger/items", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous request = %d, want 200", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityHeaders(defaultCSP))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("missing referrer policy")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}

	// CSP 置空时不下发该头。
	r2 := gin.New()
	r2.Use(securityHeaders(""))
	r2.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Header().Get("Content-Security-Policy") != "" {
		t.Fatal("empty csp should not be sent")
	}
}

func TestTokenIssuedAtInjected(t *testing.T) {
	gw := NewGateway(mustLoad(t, "auth:\n  jwt_secret: \""+testJWTSecret+"\"\n"), logger.NewNop())
	r := gin.New()
	r.Use(gw.authMiddleware())
	r.GET("/iat", func(c *gin.Context) { c.String(http.StatusOK, c.Request.Header.Get("X-Token-Iat")) })

	token := issueTestToken(t, 5)
	req := requestWithToken(http.MethodGet, "/iat", token)
	req.Header.Set("X-Token-Iat", "999") // 伪造值必须被网关覆盖
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	got := w.Body.String()
	if got == "" || got == "999" {
		t.Fatalf("X-Token-Iat = %q, want gateway-derived value", got)
	}
}
