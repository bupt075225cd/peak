package main

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	bizerr "peak/libs/errors"
	httpx "peak/libs/http"
)

// bucket 单个用户的令牌桶。
type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter 按用户维度的令牌桶限流。
//
// 只对配置的前缀（默认 /api/ledger）生效：账本为高敏感个人数据，
// 需要在网关侧挡住枚举/爆破式的高频访问；其他应用维持现状不受影响。
// 单实例内存实现，多副本部署需改为 Redis 等共享存储。
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[uint64]*bucket
	rps      float64
	burst    int
	prefixes []string
	now      func() time.Time
	lastGC   time.Time
}

func newRateLimiter(rps float64, burst int, prefixes []string) *rateLimiter {
	if rps <= 0 {
		rps = 2
	}
	if burst <= 0 {
		burst = 30
	}
	return &rateLimiter{
		buckets: map[uint64]*bucket{}, rps: rps, burst: burst,
		prefixes: prefixes, now: time.Now,
		// 首次访问时惰性初始化，便于测试注入自定义时钟。
		lastGC: time.Time{},
	}
}

// match 判断路径是否受本限流器约束。
func (rl *rateLimiter) match(path string) bool {
	for _, p := range rl.prefixes {
		if p != "" && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// allow 取一个令牌；不足时返回 false。
func (rl *rateLimiter) allow(userID uint64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	rl.gcLocked(now)

	b, ok := rl.buckets[userID]
	if !ok {
		b = &bucket{tokens: float64(rl.burst), last: now}
		rl.buckets[userID] = b
	}
	// 按经过时间补充令牌，上限为 burst。
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rl.rps
		if b.tokens > float64(rl.burst) {
			b.tokens = float64(rl.burst)
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// gcLocked 清理长时间未活跃的桶，避免内存随用户数无界增长。
// 调用方需持有 mu。
func (rl *rateLimiter) gcLocked(now time.Time) {
	if rl.lastGC.IsZero() {
		rl.lastGC = now
		return
	}
	if now.Sub(rl.lastGC) < 10*time.Minute {
		return
	}
	rl.lastGC = now
	for id, b := range rl.buckets {
		if now.Sub(b.last) > 30*time.Minute {
			delete(rl.buckets, id)
		}
	}
}

// middleware 生成限流中间件（需在鉴权中间件之后挂载，以拿到 user_id）。
func (rl *rateLimiter) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.match(c.Request.URL.Path) {
			c.Next()
			return
		}
		uid := c.GetUint64("user_id")
		if uid == 0 {
			// 鉴权中间件已拒绝的请求不会走到这里；无身份则不额外限流。
			c.Next()
			return
		}
		if !rl.allow(uid) {
			retry := int(1 / rl.rps)
			if retry < 1 {
				retry = 1
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			httpx.Fail(c, bizerr.New(bizerr.CodeRateLimited, "请求过于频繁，请稍后再试"))
			c.Abort()
			return
		}
		c.Next()
	}
}
