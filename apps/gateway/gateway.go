package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"peak/libs/auth"
	"peak/libs/config"
	bizerr "peak/libs/errors"
	httpx "peak/libs/http"
	"peak/libs/logger"
)

// Gateway 网关，按路径前缀将请求转发到对应后端服务。
type Gateway struct {
	cfg    *config.Loader
	log    *logger.Logger
	routes map[string]string // 前缀 -> 后端地址

	// 多应用隔离：路由前缀 -> 允许的 JWT aud（应用受众）列表。
	// 令牌带 aud 且不在列表内 → 403；aud 为空按 allow_legacy_tokens 决定。
	audienceRoutes map[string][]string
	// CORS 允许的前端来源白名单（精确匹配 Origin）；为空时回退 *（开发态）。
	corsOrigins []string
	// allowLegacyTokens 是否放行无 aud 的历史令牌（默认 true，收紧前置为 false）。
	allowLegacyTokens bool
	// limiter 按用户令牌桶限流（仅对配置前缀生效，默认 /api/ledger）。
	limiter *rateLimiter
	// csp Content-Security-Policy 头；空表示不下发。
	csp string
}

// NewGateway 从配置构建网关路由表。
func NewGateway(cfg *config.Loader, log *logger.Logger) *Gateway {
	routes := map[string]string{}
	// 从配置读取路由映射（routes.<prefix> = backend URL）。
	if v := cfg.Get("routes"); v != nil {
		if m, ok := v.(map[string]any); ok {
			for prefix, backend := range m {
				if s, ok := backend.(string); ok {
					routes[prefix] = s
				}
			}
		}
	}
	// 默认路由（无配置时兜底）。
	if len(routes) == 0 {
		routes["/api/questions"] = "http://localhost:8081"
		routes["/api/recognition"] = "http://localhost:8082"
		routes["/api/users"] = "http://localhost:8083"
		routes["/api/mistakes"] = "http://localhost:8081"
		routes["/api/categories"] = "http://localhost:8081"
		routes["/api/poetry"] = "http://localhost:8084"
	}
	// 应用受众路由映射（auth.audience_routes.<prefix> = [client_id...]）。
	audienceRoutes := map[string][]string{}
	if v := cfg.Get("auth.audience_routes"); v != nil {
		if m, ok := v.(map[string]any); ok {
			for prefix, raw := range m {
				if list, ok := raw.([]any); ok {
					auds := make([]string, 0, len(list))
					for _, a := range list {
						if s, ok := a.(string); ok {
							auds = append(auds, s)
						}
					}
					audienceRoutes[prefix] = auds
				}
			}
		}
	}
	// CORS 白名单（cors.allowed_origins = [origin...]），逗号分隔字符串或列表均可。
	var corsOrigins []string
	switch v := cfg.Get("cors.allowed_origins").(type) {
	case []any:
		for _, o := range v {
			if s, ok := o.(string); ok && s != "" {
				corsOrigins = append(corsOrigins, s)
			}
		}
	case string:
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				corsOrigins = append(corsOrigins, s)
			}
		}
	}
	// 限流：仅暴露给高风险前缀，默认只覆盖账本接口。
	rateLimitRPS := 2.0
	if raw := cfg.String("ratelimit.rps", ""); raw != "" {
		if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && v > 0 {
			rateLimitRPS = v
		}
	}
	limitPrefixes := splitList(cfg.String("ratelimit.prefixes", "/api/ledger"))
	limiter := newRateLimiter(rateLimitRPS, cfg.Int("ratelimit.burst", 30), limitPrefixes)

	return &Gateway{
		cfg: cfg, log: log, routes: routes,
		audienceRoutes:    audienceRoutes,
		corsOrigins:       corsOrigins,
		allowLegacyTokens: cfg.Bool("auth.allow_legacy_tokens", true),
		limiter:           limiter,
		csp:               cfg.String("security.csp", defaultCSP),
	}
}

// defaultCSP 同源策略：前端为同域 SPA，无外部 CDN 依赖。
const defaultCSP = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// splitList 解析逗号分隔配置项。
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// RegisterRoutes 注册网关路由。
func (g *Gateway) RegisterRoutes(engine *gin.Engine) {
	// 网关为最外层入口：默认不信任任何代理头（X-Forwarded-For/X-Real-IP），
	// 防止客户端伪造来源 IP 绕过限频；若上游还有可信反代（如 nginx），
	// 通过 auth.trusted_proxies 配置其地址段。
	if tp := g.cfg.String("auth.trusted_proxies", ""); tp != "" {
		_ = engine.SetTrustedProxies(strings.Split(tp, ","))
	} else {
		_ = engine.SetTrustedProxies(nil)
	}

	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})

	// 前端监控上报（公开端点，见 publicPrefixes 白名单）。
	registerMonitorReport(engine, g.log.Logger)

	// 跨域处理。
	engine.Use(g.corsMiddleware())

	// 鉴权（JWT + 应用受众）→ 按用户限流 → 安全响应头。
	engine.Use(g.authMiddleware())
	engine.Use(g.limiter.middleware())
	engine.Use(securityHeaders(g.csp))

	// 为每个前缀注册反向代理。
	for prefix, backend := range g.routes {
		g.proxy(engine, prefix, backend)
	}
}

// 公开路径白名单：登录/发码接口与健康检查不要求 JWT。
// 文件下发路径（识别产物 SVG/原图、正式区配图）同样放行：前端以
// <img src> 加载，无法携带 Authorization 头；这两个端点本身不按用户
// 隔离校验，放行仅恢复 JWT 接入前的可达性，后续可改预签名 URL 收紧。
var publicPrefixes = []string{
	"/api/users/auth/",        // 发码与验证码登录
	"/api/recognition/files/", // 识别产物文件（SVG/原图）
	"/api/mistakes/files/",    // 正式区配图（committed/）
	"/api/monitor/",           // 前端监控上报（无用户数据读取，仅写入）
	"/healthz",
	"/metrics",
}

// 按方法精确匹配的公开路由：诗歌只读接口对未登录访客开放（浏览免登录），
// POST /api/poetry/chat（智能对话）不在其列，维持 JWT 保护。
var publicMethodPrefixes = []struct {
	method string
	prefix string
}{
	{http.MethodGet, "/api/poetry/pieces"}, // 列表 / 详情 / 相关推荐
	{http.MethodGet, "/api/poetry/stats/"}, // 朝代 / 册数 / 作者 / 字词统计
}

// isPublicPath 判断请求是否在鉴权白名单内：
// 先按纯路径前缀（不限方法）匹配，再按「方法 + 路径前缀」精确匹配。
func isPublicPath(method, path string) bool {
	for _, p := range publicPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	for _, r := range publicMethodPrefixes {
		if r.method == method && strings.HasPrefix(path, r.prefix) {
			return true
		}
	}
	// 精确匹配（无尾随子路径）。
	return path == "/healthz" || path == "/metrics"
}

// authMiddleware JWT 鉴权：校验 Authorization: Bearer，以真实 user_id
// 覆盖 X-User-Id 透传给后端服务；白名单路径放行，其余未登录返回 401。
// 多应用隔离：按路由前缀校验令牌 aud（应用受众）与应用匹配，应用间令牌
// 不通用；aud 为空的历史令牌由 allow_legacy_tokens 开关决定放行策略。
// 同时为所有请求注入 X-Real-IP（取网关侧 ClientIP），供后端限流使用——
// 后端只信任网关注入值，外部直传的该头会在代理前被覆盖。
func (g *Gateway) authMiddleware() gin.HandlerFunc {
	secret := g.cfg.String("auth.jwt_secret", "")
	return func(c *gin.Context) {
		// 客户端真实 IP：网关是唯一可信入口，覆盖外部传入的头。
		c.Request.Header.Set("X-Real-IP", c.ClientIP())
		// 令牌签发时间同样只信任网关的解析结果：先剔除外部可能伪造的头。
		c.Request.Header.Del("X-Token-Iat")

		if isPublicPath(c.Request.Method, c.Request.URL.Path) {
			// 已登录用户访问公开接口时仍解析身份（尽力而为，失败不阻断）。
			if h := c.GetHeader("Authorization"); h != "" {
				if claims, err := auth.ParseClaims(strings.TrimPrefix(h, "Bearer "), secret); err == nil {
					c.Set("user_id", claims.UserID)
					c.Request.Header.Set("X-User-Id", strconv.FormatUint(claims.UserID, 10))
					injectIssuedAt(c, claims)
				}
			}
			c.Next()
			return
		}

		h := c.GetHeader("Authorization")
		if h == "" {
			httpx.Fail(c, bizerr.New(bizerr.CodeUnauthorized, "未登录或令牌缺失"))
			c.Abort()
			return
		}
		claims, err := auth.ParseClaims(strings.TrimPrefix(h, "Bearer "), secret)
		if err != nil || claims.UserID == 0 {
			g.log.Warn("invalid token", zap.String("path", c.Request.URL.Path), zap.Error(err))
			httpx.Fail(c, bizerr.New(bizerr.CodeUnauthorized, "未登录或令牌已过期"))
			c.Abort()
			return
		}
		// 应用受众校验：令牌声明的应用必须与路由允许的应用匹配（应用间令牌不通用）。
		if err := g.checkAudience(c.Request.URL.Path, claims.Audience); err != nil {
			g.log.Warn("audience mismatch", zap.String("path", c.Request.URL.Path), zap.String("aud", claims.Audience))
			httpx.Fail(c, err)
			c.Abort()
			return
		}
		// 以真实身份覆盖 X-User-Id（含外部伪造值），后端服务零改动获得用户隔离。
		c.Set("user_id", claims.UserID)
		c.Request.Header.Set("X-User-Id", strconv.FormatUint(claims.UserID, 10))
		injectIssuedAt(c, claims)
		c.Next()
	}
}

// injectIssuedAt 把令牌签发时间注入下游请求头。
//
// 账本的导出/注销等敏感操作据此判断"令牌是否新鲜"，不足则要求重新登录；
// 该头只由网关写入，客户端传入的同名头已在上游被删除。
func injectIssuedAt(c *gin.Context, claims *auth.Claims) {
	if claims.IssuedAt.IsZero() {
		return
	}
	c.Request.Header.Set("X-Token-Iat", strconv.FormatInt(claims.IssuedAt.Unix(), 10))
}

// checkAudience 校验令牌 aud 与路由前缀允许的应用受众是否匹配：
// - 路由未配置受众约束 → 放行；
// - 令牌带 aud 且在允许列表 → 放行；
// - 令牌带 aud 但不匹配 → 拒绝（应用间不可互用令牌）；
// - 令牌无 aud（历史令牌）→ 由 allow_legacy_tokens 开关决定。
func (g *Gateway) checkAudience(path, aud string) error {
	allowed, ok := g.matchAudienceRoute(path)
	if !ok {
		return nil
	}
	if aud != "" {
		for _, a := range allowed {
			if a == aud {
				return nil
			}
		}
		return bizerr.New(bizerr.CodeForbidden, "令牌不属于当前应用")
	}
	if g.allowLegacyTokens {
		return nil
	}
	return bizerr.New(bizerr.CodeUnauthorized, "令牌缺少应用受众，请重新登录")
}

// matchAudienceRoute 按最长前缀匹配路由受众配置。
func (g *Gateway) matchAudienceRoute(path string) ([]string, bool) {
	best := ""
	var allowed []string
	ok := false
	for prefix, auds := range g.audienceRoutes {
		if strings.HasPrefix(path, prefix) && len(prefix) > len(best) {
			best, allowed, ok = prefix, auds, true
		}
	}
	return allowed, ok
}

// proxy 为指定前缀创建反向代理，转发时透传 traceID。
// 后端服务注册的是完整路径（如 /api/questions/:id），
// 因此网关不剥离前缀，直接透传，保证与后端路由一致。
func (g *Gateway) proxy(engine *gin.Engine, prefix, backend string) {
	target, err := url.Parse(backend)
	if err != nil {
		g.log.Error("invalid backend", zap.String("prefix", prefix), zap.String("backend", backend))
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	handler := func(c *gin.Context) {
		// 透传追踪上下文（traceparent + X-Trace-Id 兼容）；
		// 用户身份（X-User-Id/X-Real-IP）已由鉴权中间件写入请求头。
		injectTraceContext(c)
		// gin 的 ResponseWriter 未实现 http.CloseNotifier，
		// 通过适配器补齐以兼容 httputil.ReverseProxy。
		proxy.ServeHTTP(&closeNotifyWriter{ResponseWriter: c.Writer}, c.Request)
	}

	// 同时注册前缀本身（无尾随）与子路径，保证 /api/questions 与 /api/questions/1 都能匹配。
	engine.Any(prefix, handler)
	engine.Any(prefix+"/*path", handler)
}

// injectTraceContext 向下游请求注入追踪上下文：
//   - 请求上下文有激活 span（tracing 已启用）时，写标准 W3C traceparent，
//     下游 span 以网关 span 为父，形成完整链路；
//   - 无激活 span 时，若 trace_id 为 32 位 hex 也可构造 traceparent（仅关联、不采样）；
//   - 始终透传自定义 X-Trace-Id 头，保持与旧后端/测试兼容。
func injectTraceContext(c *gin.Context) {
	tid := c.GetString("trace_id")
	spanID := func() string {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		return hex.EncodeToString(b)
	}
	if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsValid() && spanID() != "" {
		flags := "00"
		if sc.IsSampled() {
			flags = "01"
		}
		c.Request.Header.Set("traceparent",
			fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), spanID(), flags))
	} else if isHex32(tid) && spanID() != "" {
		c.Request.Header.Set("traceparent", "00-"+tid+"-"+spanID()+"-00")
	}
	if tid != "" {
		c.Request.Header.Set("X-Trace-Id", tid)
	}
}

func isHex32(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// corsMiddleware 跨域处理：独立域名多应用模型下按前端来源白名单放行
// （cors.allowed_origins，精确匹配 Origin 并回显 + Vary: Origin）；
// 未配置白名单时回退 *（本地开发态），上线必须显式配置。
func (g *Gateway) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowAll := len(g.corsOrigins) == 0
		allowed := allowAll
		if !allowAll && origin != "" {
			for _, o := range g.corsOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
		}
		if allowed {
			if allowAll || origin == "" {
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				// 白名单模式：回显具体来源，并提示缓存按 Origin 区分。
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id, X-User-Id, X-Client-Id, traceparent, tracestate")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// closeNotifyWriter 适配器：为 gin.ResponseWriter 补齐 http.CloseNotifier，
// 使 httputil.ReverseProxy 能正常转发（Go 仍会探测 CloseNotify）。
type closeNotifyWriter struct {
	http.ResponseWriter
}

// CloseNotify 返回一个永不关闭的通道，满足 http.CloseNotifier 接口。
func (w *closeNotifyWriter) CloseNotify() <-chan bool {
	return make(chan bool)
}
