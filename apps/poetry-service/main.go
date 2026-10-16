// poetry-service 古诗文服务：教材篇目语料的存储、多维检索与统计可视化数据源，
// 并预留 RAG 智能对话接口（chat，当前为契约占位）。
package main

import (
	"context"
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/apps/poetry-service/internal/handler"
	"peak/apps/poetry-service/internal/importer"
	"peak/apps/poetry-service/internal/repository"
	"peak/apps/poetry-service/internal/service"
	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
)

func main() {
	cfgPath := os.Getenv("POETRY_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	appLog := logger.New(cfg.Bool("log.development", true))

	shutdown, err := observability.InitTracer(context.Background(), "poetry-service", cfg.String("tracing.endpoint", ""))
	if err != nil {
		appLog.Error("init tracer failed", zap.String("error", err.Error()))
	}
	defer func() { _ = shutdown(context.Background()) }()

	db, err := domain.OpenDBFromConfig(cfg, gormLogLevel(cfg.Bool("log.development", true)))
	if err != nil {
		panic(err)
	}
	if err := domain.Migrate(db); err != nil {
		panic(err)
	}
	if sqlDB, derr := db.DB(); derr == nil {
		observability.RegisterDBStats(sqlDB, "poetry-service")
	}

	// 语料幂等导入：go:embed 内嵌 Markdown，按 (title, dynasty) upsert。
	total, changed, err := importer.Seed(db)
	if err != nil {
		panic(err)
	}
	appLog.Info("poetry corpus seeded", zap.Int("total", total), zap.Int("changed", changed))

	// 组装依赖：repository -> service -> handler。
	repo := repository.NewGormPoemRepository(db)
	svc := service.New(repo)
	h := handler.New(svc)

	server := httpx.NewServer(appLog, cfg.Bool("log.development", true))
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	engine.Use(observability.TracingMiddleware())
	observability.RegisterMetricsEndpoint(engine)
	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})
	h.RegisterRoutes(engine)

	addr := ":" + cfg.String("server.port", "8084")
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
