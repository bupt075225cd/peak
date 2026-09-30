package observability

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"peak/libs/logger"
)

// TracingMiddleware 为每个 HTTP 请求创建 server span（OpenTelemetry 标准）：
// 从 W3C traceparent 提取远端上下文（跨服务串联同一 trace），请求结束时
// 记录路由、状态码并关闭 span。同时把 span 的 TraceID 回写 gin 上下文的
// trace_id 与日志上下文，保证 **日志、指标、链路三者共用同一 Trace ID**；
// tracing 未启用（noop provider）时 span 无效，沿用 TraceID 中间件的兜底解析。
func TracingMiddleware() gin.HandlerFunc {
	tracer := otel.Tracer("peak/observability")
	propagator := otel.GetTextMapPropagator()
	return func(c *gin.Context) {
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		// 先以低基数名称起 span，路由匹配后再改名为具体路由模板。
		ctx, span := tracer.Start(ctx, "HTTP "+c.Request.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(c.Request.Method),
				semconv.HTTPTarget(c.Request.URL.Path),
			))
		defer func() {
			if rec := recover(); rec != nil {
				span.SetStatus(codes.Error, fmt.Sprintf("panic: %v", rec))
				span.End()
				panic(rec) // 继续抛给 Recover 中间件
			}
			if route := c.FullPath(); route != "" {
				span.SetName(route)
				span.SetAttributes(semconv.HTTPRoute(route))
			}
			code := c.Writer.Status()
			span.SetAttributes(semconv.HTTPStatusCode(code))
			if code >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", code))
			}
			span.End()
		}()
		c.Request = c.Request.WithContext(ctx)

		// trace_id 与 span 对齐（仅在 span 有效时覆盖，避免 noop 下污染兜底值）。
		if sc := span.SpanContext(); sc.IsValid() {
			tid := sc.TraceID().String()
			c.Set("trace_id", tid)
			c.Request = c.Request.WithContext(logger.WithTraceIDCtx(ctx, tid))
			c.Header("X-Trace-Id", tid)
		}
		c.Next()
	}
}
