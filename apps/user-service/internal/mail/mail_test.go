package mail

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestMockSenderLogs(t *testing.T) {
	m := NewMockSender(zap.NewNop())
	if err := m.Send("stu@peak.local", "Peak 验证码", "code 654321"); err != nil {
		t.Fatalf("mock send: %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("Peak <no-reply@peak.local>", "stu@peak.local", "Peak 验证码", "你的验证码是 654321"))

	for _, want := range []string{
		"From: Peak <no-reply@peak.local>",
		"To: stu@peak.local",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		// 中文主题需按 RFC 2047 B 编码。
		"Subject: =?UTF-8?B?",
		"你的验证码是 654321",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q should contain %q", msg, want)
		}
	}
	// 头部与正文之间以空行分隔。
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Fatal("message should contain header/body separator")
	}
}

func TestSMTPSenderConnectionError(t *testing.T) {
	// 指向不可达端口：Send 返回错误且不 panic。
	s := NewSMTPSender(SMTPConfig{
		Host: "127.0.0.1:1", From: "f@peak.local",
	})
	if err := s.Send("stu@peak.local", "s", "t"); err == nil {
		t.Fatal("unreachable smtp host should return error")
	}
}
