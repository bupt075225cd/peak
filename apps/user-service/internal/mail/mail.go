// Package mail 抽象邮件发送通道：MockSender（写日志，本地调测）与
// SMTPSender（基于 net/smtp，生产可配置）。发送验证码类邮件。
package mail

import (
	"encoding/base64"
	"fmt"
	"net/smtp"
	"strings"

	"go.uber.org/zap"
)

// Sender 邮件发送接口。
type Sender interface {
	Send(to, subject, body string) error
}

// MockSender mock 通道：不发送真实邮件，仅记录日志（本地调测用）。
type MockSender struct {
	log *zap.Logger
}

// NewMockSender 创建 mock 发送器。
func NewMockSender(log *zap.Logger) *MockSender {
	return &MockSender{log: log}
}

// Send 记录邮件日志。
func (m *MockSender) Send(to, subject, body string) error {
	m.log.Info("mock mail sent",
		zap.String("to", to),
		zap.String("subject", subject),
		zap.String("body", body))
	return nil
}

// SMTPConfig SMTP 发信配置。
type SMTPConfig struct {
	Host     string // SMTP 服务器地址（含端口），如 smtp.example.com:587
	Username string
	Password string
	From     string // 发件人地址，如 "Peak <no-reply@example.com>"
}

// SMTPSender 真实 SMTP 通道（net/smtp，自动 STARTTLS）。
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender 创建 SMTP 发送器。
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

// Send 发送纯文本邮件。smtp.SendMail 在服务器支持时自动使用 STARTTLS。
func (s *SMTPSender) Send(to, subject, body string) error {
	addr := s.cfg.Host
	host := addr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host = addr[:i]
	}
	msg := buildMessage(s.cfg.From, to, subject, body)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)
	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{to}, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// buildMessage 组装符合 RFC 5322 的纯文本邮件（Subject 按 UTF-8 编码）。
func buildMessage(from, to, subject, body string) []byte {
	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: =?UTF-8?B?%s?=\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n",
		from, to, base64.StdEncoding.EncodeToString([]byte(subject)))
	return []byte(headers + body + "\r\n")
}
