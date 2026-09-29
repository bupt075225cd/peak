package code

import (
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T, cfg Config) (*Store, *time.Time) {
	t.Helper()
	s := NewStore(cfg)
	cur := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return cur }
	return s, &cur
}

func TestGenerateReturnsSixDigits(t *testing.T) {
	s, _ := newTestStore(t, Config{})
	code, err := s.Generate("13800000001", "1.2.3.4")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code = %q, want 6 digits", code)
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			t.Fatalf("code %q contains non-digit", code)
		}
	}
}

func TestResendIntervalLimit(t *testing.T) {
	s, cur := newTestStore(t, Config{})
	if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
		t.Fatalf("first send: %v", err)
	}
	// 60 秒内重复发送被拒。
	if _, err := s.Generate("13800000001", "1.2.3.4"); err == nil || !strings.Contains(err.Error(), "频繁") {
		t.Fatalf("resend within interval: err = %v, want 频繁", err)
	}
	// 超过间隔后可再发。
	*cur = cur.Add(61 * time.Second)
	if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
		t.Fatalf("resend after interval: %v", err)
	}
}

func TestDailyLimit(t *testing.T) {
	s, cur := newTestStore(t, Config{DailyLimit: 3})
	for i := 0; i < 3; i++ {
		if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
			t.Fatalf("send %d: %v", i+1, err)
		}
		*cur = cur.Add(time.Minute) // 越过重发间隔
	}
	if _, err := s.Generate("13800000001", "1.2.3.4"); err == nil || !strings.Contains(err.Error(), "上限") {
		t.Fatalf("daily limit: err = %v, want 上限", err)
	}
	// 次日恢复。
	*cur = cur.Add(25 * time.Hour)
	if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
		t.Fatalf("send next day: %v", err)
	}
}

func TestIPHourlyLimit(t *testing.T) {
	s, cur := newTestStore(t, Config{IPHourlyLimit: 2})
	// 同一 IP 用不同手机号发送，绕过手机号维度限频也应被 IP 限制拦截。
	for i := 0; i < 2; i++ {
		phone := "1380000000" + string(rune('1'+i))
		if _, err := s.Generate(phone, "9.9.9.9"); err != nil {
			t.Fatalf("send %d: %v", i+1, err)
		}
	}
	if _, err := s.Generate("13800000003", "9.9.9.9"); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Fatalf("ip limit: err = %v, want IP", err)
	}
	// 其他 IP 不受影响。
	if _, err := s.Generate("13800000003", "8.8.8.8"); err != nil {
		t.Fatalf("other ip: %v", err)
	}
	// 窗口滑过后恢复。
	*cur = cur.Add(2 * time.Hour)
	if _, err := s.Generate("13800000004", "9.9.9.9"); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestVerifyLifecycle(t *testing.T) {
	s, cur := newTestStore(t, Config{})
	if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 错误 4 次仍可再试（未达作废阈值）。
	for i := 0; i < 4; i++ {
		if err := s.Verify("13800000001", "999999"); err == nil || !strings.Contains(err.Error(), "错误") {
			t.Fatalf("wrong verify %d: err = %v", i+1, err)
		}
	}
	// 越过重发间隔后重新生成，新码重置错误计数。
	*cur = cur.Add(61 * time.Second)
	codeStr, err := s.Generate("13800000001", "1.2.3.4")
	if err != nil {
		t.Fatalf("regenerate after attempts: %v", err)
	}
	// 正确验证成功且一次性消费。
	if err := s.Verify("13800000001", codeStr); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := s.Verify("13800000001", codeStr); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("replay verify: err = %v, want 过期", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	s, cur := newTestStore(t, Config{TTL: time.Minute})
	if _, err := s.Generate("13800000001", "1.2.3.4"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	*cur = cur.Add(2 * time.Minute)
	if err := s.Verify("13800000001", "123456"); err == nil || !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expired verify: err = %v, want 过期", err)
	}
}

func TestVerifyUnknownPhone(t *testing.T) {
	s, _ := newTestStore(t, Config{})
	if err := s.Verify("13800000000", "123456"); err == nil {
		t.Fatal("verify unknown phone should fail")
	}
}

func TestGenerateConcurrent(t *testing.T) {
	s, _ := newTestStore(t, Config{IPHourlyLimit: 1000})
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = s.Generate("13800000001", "1.2.3.4")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
