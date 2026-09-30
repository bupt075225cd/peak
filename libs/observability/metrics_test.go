package observability

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// fakeDriver 仅用于构造 *sql.DB（RegisterDBStats 只读取 Stats 计数器，不需要真实连接）。
type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("not implemented") }

func init() { sql.Register("fake-observability-test", fakeDriver{}) }

func TestObserveAICall(t *testing.T) {
	const provider, op = "test-provider", "test-op"
	beforeOK := testutil.ToFloat64(AICallTotal.WithLabelValues(provider, op, "ok"))
	beforeErr := testutil.ToFloat64(AICallTotal.WithLabelValues(provider, op, "error"))

	ObserveAICall(provider, op, nil, time.Now())
	if got := testutil.ToFloat64(AICallTotal.WithLabelValues(provider, op, "ok")); got != beforeOK+1 {
		t.Fatalf("expected ok counter +1, got %v", got)
	}

	ObserveAICall(provider, op, errors.New("boom"), time.Now())
	if got := testutil.ToFloat64(AICallTotal.WithLabelValues(provider, op, "error")); got != beforeErr+1 {
		t.Fatalf("expected error counter +1, got %v", got)
	}
}

func TestObserveMistakeOp(t *testing.T) {
	before := testutil.ToFloat64(MistakeOpsTotal.WithLabelValues("create", "ok"))
	ObserveMistakeOp("create", nil)
	if got := testutil.ToFloat64(MistakeOpsTotal.WithLabelValues("create", "ok")); got != before+1 {
		t.Fatalf("expected +1, got %v", got)
	}
	ObserveMistakeOp("create", errors.New("x"))
	if got := testutil.ToFloat64(MistakeOpsTotal.WithLabelValues("create", "error")); got < 1 {
		t.Fatalf("expected error counter >=1, got %v", got)
	}
}

func TestStartAndFinishAISpan(t *testing.T) {
	prev := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prev)
	exp := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp)))

	// 成功路径。
	ctx, span := StartAISpan(context.Background(), "p", "op_ok")
	FinishSpan(span, nil)
	// 失败路径。
	_, errSpan := StartAISpan(ctx, "p", "op_err")
	FinishSpan(errSpan, errors.New("timeout"))

	spans := exp.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Name() != "ai.op_ok" || spans[0].Status().Code != codes.Unset {
		t.Fatalf("unexpected ok span: %s %v", spans[0].Name(), spans[0].Status().Code)
	}
	if spans[1].Name() != "ai.op_err" || spans[1].Status().Code != codes.Error {
		t.Fatalf("unexpected error span: %s %v", spans[1].Name(), spans[1].Status().Code)
	}
	// client span 应带 provider/operation 属性。
	attrs := map[string]string{}
	for _, kv := range spans[1].Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if attrs["ai.provider"] != "p" || attrs["ai.operation"] != "op_err" {
		t.Fatalf("unexpected attrs: %v", attrs)
	}
}

func TestFinishSpanNil(t *testing.T) {
	// nil span 不应 panic。
	FinishSpan(nil, nil)
}

func TestTracer(t *testing.T) {
	if Tracer() == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestRegisterDBStats(t *testing.T) {
	// 假驱动 + 双重注册均不应 panic。
	db, err := sql.Open("fake-observability-test", "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	RegisterDBStats(db, "test-svc")
	RegisterDBStats(db, "test-svc") // 重复注册安全
	RegisterDBStats(nil, "nil")     // nil 连接安全
}

func TestFrontendReportCounter(t *testing.T) {
	before := testutil.ToFloat64(FrontendReportTotal.WithLabelValues("error", "js_error"))
	FrontendReportTotal.WithLabelValues("error", "js_error").Inc()
	if got := testutil.ToFloat64(FrontendReportTotal.WithLabelValues("error", "js_error")); got != before+1 {
		t.Fatalf("expected +1, got %v", got)
	}
}
