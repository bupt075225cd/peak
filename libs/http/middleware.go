package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"peak/libs/errors"
	"peak/libs/logger"
)

// TraceID 中间件：从请求中解析或生成 traceID，写入上下文并透传。
// 解析优先级：
//  1. 请求上下文中已激活的 OTel span（由 observability.TracingMiddleware 创建，
//     已从 W3C traceparent 提取远端上下文并采样）——保证日志与链路 trace 一致
//  2. W3C traceparent 头（tracing 未启用时的兜底提取）
//  3. X-Trace-Id / X-Request-Id（历史约定）
//  4. 随机生成 32 位 hex（W3C TraceID 格式）
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := TraceIDFromContext(c.Request.Context())
		if traceID == "" {
			traceID = TraceIDFromTraceparent(c.GetHeader("traceparent"))
		}
		if traceID == "" {
			traceID = c.GetHeader("X-Trace-Id")
		}
		if traceID == "" {
			traceID = c.GetHeader("X-Request-Id")
		}
		if traceID == "" {
			traceID = newTraceID()
		}
		c.Set("trace_id", traceID)
		c.Header("X-Trace-Id", traceID)
		ctx := logger.WithTraceIDCtx(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// TraceIDFromContext 从上下文中已激活的 OTel span 提取 traceID。
// 无 span（tracing 未启用）或 span 上下文无效时返回空串。
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// TraceIDFromTraceparent 解析 W3C traceparent 头，返回其中的 TraceID。
// 格式：{version}-{trace-id 32hex}-{parent-id 16hex}-{flags}，仅接受版本 00。
func TraceIDFromTraceparent(tp string) string {
	parts := strings.Split(tp, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return ""
	}
	if !isHex(parts[1], 32) || !isHex(parts[2], 16) {
		return ""
	}
	return parts[1]
}

func isHex(s string, wantLen int) bool {
	if len(s) != wantLen {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// newTraceID 生成 32 位 hex 随机 traceID（W3C TraceID 格式，可被 OTel 全链路沿用）。
func newTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下（熵源不可用）回退纳秒时间戳。
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}

// Recover 中间件：捕获 panic 并返回统一错误。
func Recover(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered", zap.Any("error", r))
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					Response{Code: int(errors.CodeInternal), Message: "internal error"})
			}
		}()
		c.Next()
	}
}

// AccessLog 中间件：记录请求访问日志。
func AccessLog(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("trace_id", c.GetString("trace_id")),
		)
	}
}
