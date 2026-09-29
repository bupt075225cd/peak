// Package code 实现短信验证码的内存存储与多维限频：
// 手机号维度（60 秒/次 + 24 小时上限）与 IP 维度（每小时上限），
// 验证码使用 crypto/rand 生成，校验使用 constant-time 比较。
package code

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// 默认安全参数。
const (
	DefaultTTL            = 5 * time.Minute  // 验证码有效期
	DefaultResendInterval = 60 * time.Second // 同号重发间隔
	DefaultDailyLimit     = 10               // 同号 24 小时内发送上限
	DefaultIPHourlyLimit  = 30               // 同 IP 每小时发送上限（防多手机号刷量）
	DefaultMaxAttempts    = 5                // 验证错误次数上限，超出作废
)

// Config 验证码存储配置，零值字段沿用默认值。
type Config struct {
	TTL            time.Duration
	ResendInterval time.Duration
	DailyLimit     int
	IPHourlyLimit  int
	MaxAttempts    int
}

func (c Config) withDefaults() Config {
	if c.TTL <= 0 {
		c.TTL = DefaultTTL
	}
	if c.ResendInterval <= 0 {
		c.ResendInterval = DefaultResendInterval
	}
	if c.DailyLimit <= 0 {
		c.DailyLimit = DefaultDailyLimit
	}
	if c.IPHourlyLimit <= 0 {
		c.IPHourlyLimit = DefaultIPHourlyLimit
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = DefaultMaxAttempts
	}
	return c
}

// entry 单个手机号的验证码记录。
type entry struct {
	code      string
	expiresAt time.Time
	attempts  int
	sentAt    []time.Time // 24 小时窗口内的发送记录（用于每日上限）
}

// Store 验证码内存存储，进程内有效（重启即清空）。
type Store struct {
	cfg   Config
	mu    sync.Mutex
	codes map[string]*entry  // phone -> entry
	ips   map[string][]time.Time // ip -> 1 小时窗口内的发送记录
	now   func() time.Time       // 可注入时钟，便于测试
}

// NewStore 创建验证码存储。
func NewStore(cfg Config) *Store {
	return &Store{
		cfg:   cfg.withDefaults(),
		codes: map[string]*entry{},
		ips:   map[string][]time.Time{},
		now:   time.Now,
	}
}

// Generate 为手机号生成并存储验证码，返回明文验证码（交由 Sender 发送）。
// 限频检查在生成之前执行，被限频的请求不产生任何状态。
func (s *Store) Generate(phone, ip string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	// IP 维度限频：清理过期后检查窗口内次数（防大量不同手机号刷量）。
	s.ips[ip] = pruneWindow(s.ips[ip], now.Add(-time.Hour))
	if len(s.ips[ip]) >= s.cfg.IPHourlyLimit {
		return "", fmt.Errorf("该 IP 请求过于频繁，请稍后再试")
	}

	// 手机号维度：60 秒重发间隔。
	if e, ok := s.codes[phone]; ok && now.Sub(lastSend(e)) < s.cfg.ResendInterval {
		return "", fmt.Errorf("发送过于频繁，请稍后再试")
	}

	// 手机号维度：24 小时发送上限。
	if e, ok := s.codes[phone]; ok {
		e.sentAt = pruneWindow(e.sentAt, now.Add(-24*time.Hour))
		if len(e.sentAt) >= s.cfg.DailyLimit {
			return "", fmt.Errorf("今日发送次数已达上限，请明天再试")
		}
	}

	code, err := generate()
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}

	// 同号重复发送时覆盖旧码，并保留 24h 发送记录。
	e, ok := s.codes[phone]
	if !ok {
		e = &entry{}
		s.codes[phone] = e
	}
	e.code = code
	e.expiresAt = now.Add(s.cfg.TTL)
	e.attempts = 0
	e.sentAt = append(e.sentAt, now)
	s.ips[ip] = append(s.ips[ip], now)

	return code, nil
}

// Verify 校验验证码：过期/不存在/错误超限/不匹配均返回错误；
// 校验成功后立即消费（防重放）。使用 constant-time 比较防时序攻击。
func (s *Store) Verify(phone, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	e, ok := s.codes[phone]
	if !ok || now.After(e.expiresAt) {
		return fmt.Errorf("验证码已过期，请重新获取")
	}
	if e.attempts >= s.cfg.MaxAttempts {
		delete(s.codes, phone)
		return fmt.Errorf("错误次数过多，验证码已作废，请重新获取")
	}
	if subtle.ConstantTimeCompare([]byte(e.code), []byte(code)) != 1 {
		e.attempts++
		if e.attempts >= s.cfg.MaxAttempts {
			delete(s.codes, phone)
			return fmt.Errorf("验证码错误次数过多已作废，请重新获取")
		}
		return fmt.Errorf("验证码错误")
	}
	delete(s.codes, phone) // 校验成功即消费，一次性使用
	return nil
}

// generate 生成 6 位数字验证码，使用 crypto/rand 密码学安全随机源。
func generate() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// lastSend 返回该记录最近一次发送时间。
func lastSend(e *entry) time.Time {
	if len(e.sentAt) == 0 {
		return time.Time{}
	}
	return e.sentAt[len(e.sentAt)-1]
}

// pruneWindow 返回仍在窗口内的时间戳，惰性清理过期项避免内存膨胀。
func pruneWindow(ts []time.Time, cutoff time.Time) []time.Time {
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	return kept
}
