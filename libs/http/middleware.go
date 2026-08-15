package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"peak/libs/errors"
	"peak/libs/logger"
)

// TraceID 中间件：从请求头提取或生成 traceID，写入上下文并透传。
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
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

// newTraceID 生成 traceID（纳秒时间戳）。
func newTraceID() string {
	return time.Now().Format("20060102150405.000000000")
}
