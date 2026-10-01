package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// 默认 Resend API 端点与请求超时。
const (
	DefaultResendURL     = "https://api.resend.com/emails"
	DefaultResendTimeout = 10 * time.Second
	maxErrorBodyChars    = 256 // 错误信息中截断的响应体长度，避免日志放大
)

// ResendConfig Resend 发信配置；BaseURL/Client 零值时使用默认值（便于测试注入）。
type ResendConfig struct {
	APIKey  string
	From    string
	BaseURL string       // 默认 https://api.resend.com/emails
	Client  *http.Client // 默认 10s 超时
}

// resendPayload Resend 发信请求体（纯文本，验证码类短内容）。
type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// ResendSender 基于 Resend REST API 的发信通道
// （POST /emails，Bearer API Key 鉴权，零新依赖）。
type ResendSender struct {
	cfg    ResendConfig
	client *http.Client
	url    string
}

// NewResendSender 创建 Resend 发送器。
func NewResendSender(cfg ResendConfig) *ResendSender {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: DefaultResendTimeout}
	}
	url := cfg.BaseURL
	if url == "" {
		url = DefaultResendURL
	}
	return &ResendSender{cfg: cfg, client: client, url: url}
}

// Send 发送纯文本邮件：POST JSON {from, to:[to], subject, text}，
// 2xx 视为成功，非 2xx 返回含响应摘要的错误（API Key 不出现在错误信息中）。
func (s *ResendSender) Send(to, subject, body string) error {
	payload, err := json.Marshal(resendPayload{
		From:    s.cfg.From,
		To:      []string{to},
		Subject: subject,
		Text:    body,
	})
	if err != nil {
		return fmt.Errorf("resend marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("resend do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, truncateBody(resp.Body))
	}
	// 消费并丢弃成功响应体，复用底层连接。
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// truncateBody 读取响应体并截断为摘要字符串。
func truncateBody(r io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(r, maxErrorBodyChars*4))
	if err != nil {
		return "<unreadable>"
	}
	text := string(bytes.TrimSpace(raw))
	if len(text) > maxErrorBodyChars {
		text = text[:maxErrorBodyChars]
	}
	return text
}
