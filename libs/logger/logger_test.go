package logger

import (
	"context"
	"testing"
)

func TestNewNop(t *testing.T) {
	l := NewNop()
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	// nop logger 调用不应 panic。
	l.Info("test")
	l.Error("test")
}

func TestNewDevelopment(t *testing.T) {
	l := New(true)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewProduction(t *testing.T) {
	l := New(false)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestWithTraceID(t *testing.T) {
	l := NewNop()
	got := l.WithTraceID("trace-123")
	if got == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestWithCtx(t *testing.T) {
	l := NewNop()

	// nil ctx 返回自身。
	if got := l.WithCtx(nil); got != l { //nolint:staticcheck // 有意验证 nil ctx 行为
		t.Fatal("expected same logger for nil ctx")
	}

	// 无 traceID 的 ctx 返回自身。
	if got := l.WithCtx(context.Background()); got != l {
		t.Fatal("expected same logger for ctx without traceID")
	}

	// 有 traceID 的 ctx 返回新 logger。
	ctx := WithTraceIDCtx(context.Background(), "trace-abc")
	got := l.WithCtx(ctx)
	if got == l {
		t.Fatal("expected new logger for ctx with traceID")
	}
}

func TestWithTraceIDCtx(t *testing.T) {
	ctx := WithTraceIDCtx(context.Background(), "t-1")
	if ctx == nil {
		t.Fatal("expected non-nil ctx")
	}
}
