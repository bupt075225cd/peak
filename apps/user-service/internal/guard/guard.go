// Package guard 实现密码类认证接口（密码登录/密码重置）的防暴力破解：
// 账号维度连续失败锁定（5 次锁 15 分钟）与 IP 维度滑动窗口限频（15 分钟 20 次）。
// 进程内内存实现（重启即清空），与 internal/code.Store 同一模式：
// sync.Mutex + map + 滑动窗口惰性清理 + 可注入时钟便于测试。
package guard

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 默认防爆破参数。
const (
	DefaultMaxFails     = 5                // 连续失败上限
	DefaultLockDuration = 15 * time.Minute // 触发上限后的锁定时长
	DefaultIPWindow     = 15 * time.Minute // IP 限频滑动窗口
	DefaultIPMaxAttempts = 20              // 窗口内最大尝试次数（成败均计）
)

// Config 防爆破参数，零值字段沿用默认值。
type Config struct {
	MaxFails     int
	LockDuration time.Duration
	IPWindow     time.Duration
	IPMaxAttempts int
}

func (c Config) withDefaults() Config {
	if c.MaxFails <= 0 {
		c.MaxFails = DefaultMaxFails
	}
	if c.LockDuration <= 0 {
		c.LockDuration = DefaultLockDuration
	}
	if c.IPWindow <= 0 {
		c.IPWindow = DefaultIPWindow
	}
	if c.IPMaxAttempts <= 0 {
		c.IPMaxAttempts = DefaultIPMaxAttempts
	}
	return c
}

// normalizeKey 账号规范化：去空白并转小写（邮箱不区分大小写，手机号不受影响）。
func normalizeKey(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

// Guard 防爆破器：组合账号锁定与 IP 限频，密码登录/重置共用同一实例。
type Guard struct {
	cfg      Config
	accounts *AccountLocker
	ips      *IPLimiter
}

// NewGuard 创建防爆破器。
func NewGuard(cfg Config) *Guard {
	c := cfg.withDefaults()
	return &Guard{
		cfg:      c,
		accounts: NewAccountLocker(c.MaxFails, c.LockDuration),
		ips:      NewIPLimiter(c.IPWindow, c.IPMaxAttempts),
	}
}

// Check 在认证尝试前调用：账号锁定中或 IP 超限均拒绝。
// 返回 nil 表示放行，否则返回面向用户的拒绝原因（不含账号是否存在的信息）。
func (g *Guard) Check(account, ip string) error {
	if remaining, locked := g.accounts.Check(account); locked {
		return &LockedError{RetryAfter: remaining}
	}
	if !g.ips.Allow(ip) {
		return ErrIPRateLimited
	}
	return nil
}

// OnFailure 认证失败时调用：累计账号失败次数（可能触发锁定）。
// IP 窗口已在 Check 中计数，此处不重复计入。返回本次失败是否触发了账号锁定。
func (g *Guard) OnFailure(account, _ string) bool {
	return g.accounts.Fail(account)
}

// OnSuccess 认证成功时调用：清零账号连续失败计数（IP 窗口不清，保持总尝试约束）。
func (g *Guard) OnSuccess(account string) {
	g.accounts.Reset(account)
}

// LockedError 账号锁定中，携带建议重试间隔。
type LockedError struct {
	RetryAfter time.Duration
}

func (e *LockedError) Error() string {
	// 向上取整：避免因校验耗时把剩余 14.98 分钟显示成 14 分钟。
	minutes := int(math.Ceil(e.RetryAfter.Minutes()))
	if minutes < 1 {
		minutes = 1
	}
	return "尝试次数过多，账号已锁定，请约 " + strconv.Itoa(minutes) + " 分钟后再试"
}

// ErrIPRateLimited 同一 IP 短时间内尝试过多。
var ErrIPRateLimited = errString("操作过于频繁，请稍后再试")

type errString string

func (e errString) Error() string { return string(e) }

// accountEntry 单个账号的失败记录。
type accountEntry struct {
	fails       int
	lockedUntil time.Time
}

// AccountLocker 账号维度连续失败锁定器。对不存在的账号同样计数，
// 避免通过"是否触发锁定"探测账号存在性。
type AccountLocker struct {
	maxFails     int
	lockDuration time.Duration
	mu           sync.Mutex
	accounts     map[string]*accountEntry
	now          func() time.Time // 可注入时钟，便于测试
}

// NewAccountLocker 创建账号锁定器。
func NewAccountLocker(maxFails int, lockDuration time.Duration) *AccountLocker {
	return &AccountLocker{
		maxFails:     maxFails,
		lockDuration: lockDuration,
		accounts:     map[string]*accountEntry{},
		now:          time.Now,
	}
}

// Check 返回该账号是否处于锁定中及剩余锁定时长。
func (l *AccountLocker) Check(account string) (time.Duration, bool) {
	key := normalizeKey(account)
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.accounts[key]
	if !ok {
		return 0, false
	}
	if remaining := e.lockedUntil.Sub(l.now()); remaining > 0 {
		return remaining, true
	}
	// 锁定已过期：清零计数重新累计，并惰性移除空条目防内存膨胀。
	if e.lockedUntil != (time.Time{}) {
		e.fails = 0
		e.lockedUntil = time.Time{}
	}
	if e.fails == 0 {
		delete(l.accounts, key)
	}
	return 0, false
}

// Fail 累计一次失败，达到上限时触发锁定，返回是否本次触发。
func (l *AccountLocker) Fail(account string) bool {
	key := normalizeKey(account)
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.accounts[key]
	if !ok {
		e = &accountEntry{}
		l.accounts[key] = e
	}
	// 已过锁定期的旧条目重新累计。
	if e.lockedUntil != (time.Time{}) && !l.now().Before(e.lockedUntil) {
		e.fails = 0
		e.lockedUntil = time.Time{}
	}
	e.fails++
	if e.fails >= l.maxFails {
		e.lockedUntil = l.now().Add(l.lockDuration)
		return true
	}
	return false
}

// Reset 认证成功后清零失败计数。
func (l *AccountLocker) Reset(account string) {
	key := normalizeKey(account)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.accounts, key)
}

// IPLimiter IP 维度滑动窗口限频器：窗口内所有尝试（无论成败）均计数。
type IPLimiter struct {
	window     time.Duration
	maxAttempts int
	mu         sync.Mutex
	ips        map[string][]time.Time
	now        func() time.Time // 可注入时钟，便于测试
}

// NewIPLimiter 创建 IP 限频器。
func NewIPLimiter(window time.Duration, maxAttempts int) *IPLimiter {
	return &IPLimiter{
		window:      window,
		maxAttempts: maxAttempts,
		ips:         map[string][]time.Time{},
		now:         time.Now,
	}
}

// Allow 记录一次尝试并判断是否放行（滑动窗口内超限拒绝；被拒的尝试同样计入）。
func (l *IPLimiter) Allow(ip string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.ips[ip][:0]
	for _, t := range l.ips[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.ips[ip] = append(kept, now)
	return len(l.ips[ip]) <= l.maxAttempts
}
