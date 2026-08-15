// Package http 提供统一的 HTTP 响应封装、中间件与服务启动助手。
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"peak/libs/errors"
)

// Response 统一响应结构。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok", Data: data})
}

// Fail 根据错误返回失败响应。
func Fail(c *gin.Context, err error) {
	be := errors.From(err)
	status := httpStatus(be.Code)
	c.JSON(status, Response{Code: int(be.Code), Message: be.Message})
}

// httpStatus 将业务错误码映射为 HTTP 状态码。
func httpStatus(code errors.Code) int {
	switch code {
	case errors.CodeInvalidArgument:
		return http.StatusBadRequest
	case errors.CodeUnauthorized:
		return http.StatusUnauthorized
	case errors.CodeForbidden:
		return http.StatusForbidden
	case errors.CodeNotFound:
		return http.StatusNotFound
	case errors.CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
