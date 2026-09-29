// Package sms 抽象短信发送通道：当前仅提供 MockSender（写日志），
// 下一迭代接入真实短信服务商时新增 Sender 实现即可。
package sms

import "go.uber.org/zap"

// Sender 短信发送接口。
type Sender interface {
	Send(phone, code string) error
}

// MockSender mock 通道：不发送真实短信，仅记录日志（本地调测用）。
type MockSender struct {
	log *zap.Logger
}

// NewMockSender 创建 mock 发送器。
func NewMockSender(log *zap.Logger) *MockSender {
	return &MockSender{log: log}
}

// Send 记录验证码日志。
func (m *MockSender) Send(phone, code string) error {
	m.log.Info("mock sms sent", zap.String("phone", phone), zap.String("code", code))
	return nil
}
