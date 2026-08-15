// gateway 是 API 网关服务，负责统一入口、路由转发、鉴权（预留）、跨域与链路透传。
package main

import (
	"context"
	"log"
	"os"

	"go.uber.org/zap"

	"peak/libs/config"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
)

func main() {
	cfgPath := os.Getenv("GATEWAY_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log := logger.New(cfg.Bool("log.development", true))

	// 初始化链路追踪（endpoint 为空时为 no-op）。
	shutdown, err := observability.InitTracer(context.Background(), "gateway", cfg.String("tracing.endpoint", ""))
	if err != nil {
		log.Error("init tracer failed", zap.String("error", err.Error()))
	}
	defer func() { _ = shutdown(context.Background()) }()

	server := httpx.NewServer(log, cfg.Bool("log.development", true))

	// 接入 Prometheus 指标。
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	observability.RegisterMetricsEndpoint(engine)

	gw := NewGateway(cfg, log)
	gw.RegisterRoutes(engine)

	addr := ":" + cfg.String("server.port", "8080")
	if err := server.Run(addr); err != nil {
		log.Error("server exit", zap.String("error", err.Error()))
	}
}
