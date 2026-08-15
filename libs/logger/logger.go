// Package logger 基于 zap 提供统一的结构化日志封装。
package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceIDKey 上下文与日志中 traceID 的统一字段名。
const TraceIDKey = "trace_id"

type ctxKey struct{}

// Logger 日志封装。
type Logger struct {
	*zap.Logger
}

// New 根据是否开发模式创建日志器。
func New(development bool) *Logger {
	var cfg zap.Config
	if development {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	core, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return &Logger{core}
}

// NewNop 返回不输出任何内容的日志器，用于测试。
func NewNop() *Logger {
	return &Logger{zap.NewNop()}
}

// WithTraceID 为日志附加 traceID 字段。
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{l.Logger.With(zap.String(TraceIDKey, traceID))}
}

// WithCtx 从上下文提取 traceID 并附加。
func (l *Logger) WithCtx(ctx context.Context) *Logger {
	if ctx == nil {
		return l
	}
	if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
		return l.WithTraceID(v)
	}
	return l
}

// WithTraceIDCtx 将 traceID 写入上下文。
func WithTraceIDCtx(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, traceID)
}
