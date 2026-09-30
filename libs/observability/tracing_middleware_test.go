package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestTracingMiddlewareSpan 测试入口 span：远端 traceparent 提取、路由改名、
// gin 上下文 trace_id 与 span TraceID 对齐。
func TestTracingMiddlewareSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 隔离全局 tracer provider，避免污染其他测试。
	prev := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prev)

	exp := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var gotTraceID, remoteTraceID string
	r := gin.New()
	r.Use(TracingMiddleware())
	r.GET("/api/demo/:id", func(c *gin.Context) {
		gotTraceID = c.GetString("trace_id")
		// handler 内请求上下文应包含激活 span。
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsValid() {
			remoteTraceID = sc.TraceID().String()
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/demo/1", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// handler 内 span 与 gin trace_id 均沿用远端 TraceID。
	if remoteTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected remote trace id in request context, got %q", remoteTraceID)
	}
	if gotTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected gin trace_id aligned with span, got %q", gotTraceID)
	}
	if w.Header().Get("X-Trace-Id") != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatal("expected X-Trace-Id header aligned with span trace id")
	}

	spans := exp.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("span trace id mismatch: %s", span.SpanContext().TraceID())
	}
	if span.Name() != "/api/demo/:id" {
		t.Fatalf("expected span renamed to route template, got %q", span.Name())
	}
	if span.SpanKind() != trace.SpanKindServer {
		t.Fatal("expected server span kind")
	}
}

// TestTracingMiddlewareNoop 测试 tracing 未启用（noop provider）时不覆盖
// TraceID 中间件的兜底解析，且不 panic。
func TestTracingMiddlewareNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prev := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prev)
	otel.SetTracerProvider(nooptrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	r := gin.New()
	r.Use(TracingMiddleware())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestTracingMiddlewareErrorStatus 测试 5xx 时 span 记录错误状态。
func TestTracingMiddlewareErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prev := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prev)

	exp := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp)))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	r := gin.New()
	r.Use(TracingMiddleware())
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	spans := exp.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if codes.Error != spans[0].Status().Code {
		t.Fatalf("expected error status, got %v", spans[0].Status().Code)
	}
}
