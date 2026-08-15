// recognition-service 是识别服务，封装第三方 OCR/手写擦除/公式/几何识别，异步任务处理。
package main

import (
	"context"
	"os"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
	"peak/libs/storage"

	"peak/apps/recognition-service/internal/handler"
	"peak/apps/recognition-service/internal/provider"
	"peak/apps/recognition-service/internal/service"
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

	// 初始化数据库。
	db, err := domain.OpenDB(
		domain.DBDialect(cfg.String("database.dialect", "mysql")),
		cfg.String("database.dsn", ""),
		gormLogLevel(cfg.Bool("log.development", true)),
	)
	if err != nil {
		panic(err)
	}
	if err := domain.Migrate(db); err != nil {
		panic(err)
	}

	// 初始化存储。
	store, err := storage.NewLocalStorage(cfg.String("storage.root", "./data"))
	if err != nil {
		panic(err)
	}

	// 初始化识别 provider（可配置切换）。
	prov, err := provider.NewFromConfig(cfg)
	if err != nil {
		panic(err)
	}
	appLog.Info("recognition provider loaded", zap.String("provider", prov.Name()))

	// 组装依赖。
	svc := service.New(db, store, prov, appLog)
	h := handler.New(svc, db, store)

	server := httpx.NewServer(appLog, cfg.Bool("log.development", true))
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	observability.RegisterMetricsEndpoint(engine)
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
