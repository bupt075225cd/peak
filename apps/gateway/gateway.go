package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"peak/libs/config"
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
	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})

	// 跨域处理。
	engine.Use(corsMiddleware())

	// 预留鉴权中间件（当前为透传，后续接入 user-service）。
	engine.Use(g.authMiddleware())

	// 为每个前缀注册反向代理。
	for prefix, backend := range g.routes {
		g.proxy(engine, prefix, backend)
	}
}

// authMiddleware 预留鉴权：当前放行并写入 mock 用户，后续校验 JWT。
func (g *Gateway) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 预留：从 Authorization 头解析用户，当前使用 mock 用户。
		userID := c.GetHeader("X-User-Id")
		if userID == "" {
			userID = "mock-user-1"
		}
		c.Set("user_id", userID)
		c.Request.Header.Set("X-User-Id", userID)
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
		// 透传 traceID 与用户信息。
		if tid := c.GetString("trace_id"); tid != "" {
			c.Request.Header.Set("X-Trace-Id", tid)
		}
		if uid := c.GetString("user_id"); uid != "" {
			c.Request.Header.Set("X-User-Id", uid)
		}
		// gin 的 ResponseWriter 未实现 http.CloseNotifier，
		// 通过适配器补齐以兼容 httputil.ReverseProxy。
		proxy.ServeHTTP(&closeNotifyWriter{ResponseWriter: c.Writer}, c.Request)
	}

	// 同时注册前缀本身（无尾随）与子路径，保证 /api/questions 与 /api/questions/1 都能匹配。
	engine.Any(prefix, handler)
	engine.Any(prefix+"/*path", handler)
}

// corsMiddleware 跨域处理（开发阶段全放开，生产通过配置收紧）。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-Id, X-User-Id")
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
