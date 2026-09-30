// recognition-service 是识别服务，封装第三方 OCR/手写擦除/公式/几何识别，异步任务处理。
package main

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/apps/recognition-service/internal/handler"
	"peak/apps/recognition-service/internal/provider"
	"peak/apps/recognition-service/internal/service"
	storagefactory "peak/apps/recognition-service/internal/storage"
	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
)

func main() {
	cfgPath := os.Getenv("RECOGNITION_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	appLog := logger.New(cfg.Bool("log.development", true))

	shutdown, err := observability.InitTracer(context.Background(), "recognition-service", cfg.String("tracing.endpoint", ""))
	if err != nil {
		appLog.Error("init tracer failed", zap.String("error", err.Error()))
	}
	defer func() { _ = shutdown(context.Background()) }()

	// 初始化数据库（连接池 + 慢查询日志 + OTel SQL 追踪，均由配置驱动）。
	db, err := domain.OpenDBFromConfig(cfg, gormLogLevel(cfg.Bool("log.development", true)))
	if err != nil {
		panic(err)
	}
	if err := domain.Migrate(db); err != nil {
		panic(err)
	}
	// 暴露连接池指标（连接数/等待数/等待时长），用于告警连接池饱和。
	if sqlDB, derr := db.DB(); derr == nil {
		observability.RegisterDBStats(sqlDB, "recognition-service")
	}

	// 初始化存储：local=本地磁盘（默认，本地调试）/ s3=S3 兼容对象存储（Docker/K8s 部署）。
	store, err := storagefactory.New(cfg)
	if err != nil {
		panic(err)
	}
	appLog.Info("storage backend loaded", zap.String("type", storagefactory.Type(cfg)))

	// 初始化识别 provider（可配置切换）。
	prov, err := provider.NewFromConfig(cfg)
	if err != nil {
		panic(err)
	}
	appLog.Info("recognition provider loaded", zap.String("provider", prov.Name()))

	// 几何重绘（可选）：geometry.enabled 为 true 时启用内置 Go 渲染器
	// （internal/geom：坐标直出 + 文字避让 + 手写 SVG），不再依赖外部渲染服务。
	var svcOpts []service.Option
	if cfg.Bool("geometry.enabled", false) {
		maxAttempts := cfg.Int("geometry.max_attempts", 3)
		svcOpts = append(svcOpts, service.WithGeometryRender(true, maxAttempts))
		appLog.Info("geometry redraw enabled", zap.Int("max_attempts", maxAttempts))
	} else {
		appLog.Info("geometry redraw disabled (geometry.enabled is false)")
	}

	// 组装依赖。
	svc := service.New(db, store, prov, appLog, svcOpts...)

	// transient/ 临时区的清理由对象存储生命周期规则完成（按前缀 + 对象年龄过期），
	// 应用层不再做周期 GC；本地存储调试时产物残留可忽略。

	// 文件访问签名密钥：与网关 JWT 密钥同源（仅本地签名用途，不做 JWT 校验）。
	fileSecret := cfg.String("files.sign_secret", "")
	h := handler.New(svc, db, store, fileSecret)

	server := httpx.NewServer(appLog, cfg.Bool("log.development", true))
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	engine.Use(observability.TracingMiddleware())
	observability.RegisterMetricsEndpoint(engine)
	// 存活探针（轻量，不探测 DB；依赖健康由容器编排 restart 策略兜底）。
	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})
	h.RegisterRoutes(engine)

	addr := ":" + cfg.String("server.port", "8082")
	if err := server.Run(addr); err != nil {
		appLog.Error("server exit", zap.String("error", err.Error()))
	}
}

func gormLogLevel(dev bool) gormlogger.LogLevel {
	if dev {
		return gormlogger.Info
	}
	return gormlogger.Warn
}
