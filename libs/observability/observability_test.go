package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStatusLabel(t *testing.T) {
	cases := map[int]string{
		200: "2xx",
		204: "2xx",
		301: "2xx",
		404: "4xx",
		499: "4xx",
		500: "5xx",
		502: "5xx",
	}
	for code, want := range cases {
		if got := statusLabel(code); got != want {
			t.Fatalf("statusLabel(%d) = %s, want %s", code, got, want)
		}
	}
}

func TestMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MetricsMiddleware())
	r.GET("/hello", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/err", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/err", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
}

func TestRegisterMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterMetricsEndpoint(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInitTracerNoop(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), "svc", "")
	if err != nil {
		t.Fatalf("noop tracer should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown should not error: %v", err)
	}
}

func TestSpanAttribute(t *testing.T) {
	kv := SpanAttribute("k", "v")
	if string(kv.Key) != "k" || kv.Value.AsString() != "v" {
		t.Fatal("unexpected attribute")
	}
}
