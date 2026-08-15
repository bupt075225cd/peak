// Package errors 定义统一的业务错误码与错误类型，供所有服务复用。
package errors

import (
	"errors"
	"fmt"
)

// Code 业务错误码。
type Code int

// 通用错误码段。
const (
	CodeOK               Code = 0
	CodeInvalidArgument  Code = 1001
	CodeUnauthorized     Code = 1002
	CodeForbidden        Code = 1003
	CodeNotFound         Code = 1004
	CodeConflict         Code = 1005
	CodeInternal         Code = 5000
	CodeUpstream         Code = 5001
	CodeRecognitionFail  Code = 5002
	CodeStorageFail      Code = 5003
)

// Error 业务错误，包含错误码与消息。
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is / errors.As 链。
func (e *Error) Unwrap() error {
	return e.Cause
}

// New 创建业务错误。
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 包装底层错误。
func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// From 将任意 error 转换为 *Error，非业务错误归类为内部错误。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var be *Error
	if errors.As(err, &be) {
		return be
	}
	return &Error{Code: CodeInternal, Message: err.Error(), Cause: err}
}

// CodeOf 提取错误码，非业务错误返回 CodeInternal。
func CodeOf(err error) Code {
	if err == nil {
		return CodeOK
	}
	return From(err).Code
}
