// question-service 是题目/错题核心领域服务，负责错题、题目、分类的 CRUD 与手动修正。
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

	"peak/apps/question-service/internal/handler"
	"peak/apps/question-service/internal/repository"
	"peak/apps/question-service/internal/service"
)

func main() {
	cfgPath := os.Getenv("QUESTION_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	appLog := logger.New(cfg.Bool("log.development", true))

	shutdown, err := observability.InitTracer(context.Background(), "question-service", cfg.String("tracing.endpoint", ""))
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

	// 组装依赖：repository -> service -> handler。
	repos := repository.NewGormRepositories(db)
	svc := service.New(repos)
	h := handler.New(svc)

	server := httpx.NewServer(appLog, cfg.Bool("log.development", true))
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	observability.RegisterMetricsEndpoint(engine)
	h.RegisterRoutes(engine)

	addr := ":" + cfg.String("server.port", "8081")
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
