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
	}
	return &Gateway{cfg: cfg, log: log, routes: routes}
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
	engine.Use(corsMiddleware())

	// 预留鉴权中间件（当前为透传，后续接入 user-service）。
	engine.Use(g.authMiddleware())

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

// isPublicPath 判断路径是否在鉴权白名单内。
func isPublicPath(path string) bool {
	for _, p := range publicPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// 精确匹配（无尾随子路径）。
	return path == "/healthz" || path == "/metrics"
}

// authMiddleware JWT 鉴权：校验 Authorization: Bearer，以真实 user_id
// 覆盖 X-User-Id 透传给后端服务；白名单路径放行，其余未登录返回 401。
// 同时为所有请求注入 X-Real-IP（取网关侧 ClientIP），供后端限流使用——
// 后端只信任网关注入值，外部直传的该头会在代理前被覆盖。
func (g *Gateway) authMiddleware() gin.HandlerFunc {
	secret := g.cfg.String("auth.jwt_secret", "")
	return func(c *gin.Context) {
		// 客户端真实 IP：网关是唯一可信入口，覆盖外部传入的头。
		c.Request.Header.Set("X-Real-IP", c.ClientIP())

		if isPublicPath(c.Request.URL.Path) {
			// 已登录用户访问公开接口时仍解析身份（尽力而为，失败不阻断）。
			if h := c.GetHeader("Authorization"); h != "" {
				if uid, err := auth.Parse(strings.TrimPrefix(h, "Bearer "), secret); err == nil {
					c.Set("user_id", uid)
					c.Request.Header.Set("X-User-Id", strconv.FormatUint(uid, 10))
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
		uid, err := auth.Parse(strings.TrimPrefix(h, "Bearer "), secret)
		if err != nil || uid == 0 {
			g.log.Warn("invalid token", zap.String("path", c.Request.URL.Path), zap.Error(err))
			httpx.Fail(c, bizerr.New(bizerr.CodeUnauthorized, "未登录或令牌已过期"))
			c.Abort()
			return
		}
		// 以真实身份覆盖 X-User-Id（含外部伪造值），后端服务零改动获得用户隔离。
		c.Set("user_id", uid)
		c.Request.Header.Set("X-User-Id", strconv.FormatUint(uid, 10))
		c.Next()
	}
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
// - 请求上下文有激活 span（tracing 已启用）时，写标准 W3C traceparent，
//   下游 span 以网关 span 为父，形成完整链路；
// - 无激活 span 时，若 trace_id 为 32 位 hex 也可构造 traceparent（仅关联、不采样）；
// - 始终透传自定义 X-Trace-Id 头，保持与旧后端/测试兼容。
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

// corsMiddleware 跨域处理（开发阶段全放开，生产通过配置收紧）。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id, X-User-Id, traceparent, tracestate")
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
