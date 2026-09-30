package guard

import (
	stderrors "errors"
	"testing"
	"time"
)

// fakeClock 可推进的注入时钟。
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func TestAccountLockerLockAfterMaxFails(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	l := NewAccountLocker(5, 15*time.Minute)
	l.now = clk.Now

	for i := 1; i <= 4; i++ {
		if locked := l.Fail("stu@peak.local"); locked {
			t.Fatalf("fail %d should not trigger lock", i)
		}
		if _, locked := l.Check("stu@peak.local"); locked {
			t.Fatalf("fail %d: account should not be locked", i)
		}
	}
	if locked := l.Fail("stu@peak.local"); !locked {
		t.Fatal("5th fail should trigger lock")
	}
	remaining, locked := l.Check("stu@peak.local")
	if !locked || remaining <= 0 || remaining > 15*time.Minute {
		t.Fatalf("locked remaining = %v, locked = %v", remaining, locked)
	}
}

func TestAccountLockerUnlockAfterDuration(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	l := NewAccountLocker(2, 15*time.Minute)
	l.now = clk.Now

	l.Fail("a@peak.local")
	l.Fail("a@peak.local")
	if _, locked := l.Check("a@peak.local"); !locked {
		t.Fatal("should be locked after 2 fails")
	}

	// 锁定期间失败计数继续累计但不重复延长？——当前实现：Fail 重新累计并再次锁定。
	// 推进到锁定期过后：计数清零，可重新尝试。
	clk.Advance(15 * time.Minute)
	if _, locked := l.Check("a@peak.local"); locked {
		t.Fatal("lock should expire after duration")
	}
	l.Fail("a@peak.local")
	if _, locked := l.Check("a@peak.local"); locked {
		t.Fatal("single fail after expiry should not lock")
	}
}

func TestAccountLockerSuccessResets(t *testing.T) {
	l := NewAccountLocker(3, time.Minute)
	for i := 0; i < 2; i++ {
		l.Fail("b@peak.local")
	}
	l.Reset("b@peak.local")
	for i := 0; i < 2; i++ {
		if locked := l.Fail("b@peak.local"); locked {
			t.Fatalf("fail %d after reset should not lock", i+1)
		}
	}
}

func TestAccountLockerNormalizesKey(t *testing.T) {
	l := NewAccountLocker(2, time.Minute)
	l.Fail("Stu@Peak.Local ")
	l.Fail("stu@peak.local")
	if _, locked := l.Check("STU@peak.local"); !locked {
		t.Fatal("case-insensitive account should be locked")
	}
}

func TestIPLimiterSlidingWindow(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	l := NewIPLimiter(time.Minute, 3)
	l.now = clk.Now

	for i := 0; i < 3; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("4th attempt should be rejected")
	}
	// 独立 IP 不受影响。
	if !l.Allow("10.0.0.2") {
		t.Fatal("other IP should be allowed")
	}
	// 窗口滑出后恢复。
	clk.Advance(time.Minute)
	if !l.Allow("10.0.0.1") {
		t.Fatal("attempt after window should be allowed")
	}
}

func TestGuardCheckAndFailureFlow(t *testing.T) {
	clk := &fakeClock{t: time.Now()}
	g := NewGuard(Config{MaxFails: 3, LockDuration: 10 * time.Minute, IPWindow: time.Minute, IPMaxAttempts: 100})
	g.accounts.now = clk.Now
	g.ips.now = clk.Now

	// 初始放行。
	if err := g.Check("c@peak.local", "10.1.1.1"); err != nil {
		t.Fatalf("initial check: %v", err)
	}
	// 连续失败至触发锁定。
	for i := 0; i < 3; i++ {
		g.OnFailure("c@peak.local", "10.1.1.1")
	}
	err := g.Check("c@peak.local", "10.1.1.1")
	if err == nil {
		t.Fatal("check should reject locked account")
	}
	var le *LockedError
	if !stderrors.As(err, &le) {
		t.Fatalf("want LockedError, got %T: %v", err, le)
	}
	// 成功后清零解锁。
	clk.Advance(10 * time.Minute)
	g.OnSuccess("c@peak.local")
	if err := g.Check("c@peak.local", "10.1.1.1"); err != nil {
		t.Fatalf("check after reset: %v", err)
	}
}

func TestGuardIPRateLimited(t *testing.T) {
	g := NewGuard(Config{MaxFails: 100, IPWindow: time.Minute, IPMaxAttempts: 2})
	// 每次认证尝试前调用 Check（计入 IP 窗口）：第 3 次超限拒绝。
	if err := g.Check("x@peak.local", "10.2.2.2"); err != nil {
		t.Fatalf("first check: %v", err)
	}
	g.OnFailure("x@peak.local", "10.2.2.2")
	if err := g.Check("y@peak.local", "10.2.2.2"); err != nil {
		t.Fatalf("second check: %v", err)
	}
	err := g.Check("z@peak.local", "10.2.2.2")
	if !stderrors.Is(err, ErrIPRateLimited) {
		t.Fatalf("want ErrIPRateLimited, got %v", err)
	}
}
