// asset-service 家庭资产账本服务：加密存储家庭资产与负债、月度快照、
// 资产组成聚合，并提供同意留痕、导出与密码学删除等隐私能力。
package main

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/apps/asset-service/internal/handler"
	"peak/apps/asset-service/internal/repository"
	"peak/apps/asset-service/internal/service"
	"peak/apps/asset-service/internal/sweeper"
	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
	"peak/libs/security"
)

func main() {
	cfgPath := os.Getenv("ASSET_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	dev := cfg.Bool("log.development", true)
	appLog := logger.New(dev)

	// 密钥 fail-fast：生产缺少密钥或仍使用内置开发密钥时拒绝启动，
	// 避免带着弱密钥上线（配置错误比"能跑起来"更重要）。
	keys, err := security.LoadKeys(security.KeyConfig{
		MasterKeys: masterKeys(cfg),
		IDKey:      cfg.String("security.ledger_id_key", ""),
		MACKey:     cfg.String("security.ledger_mac_key", ""),
		Version:    cfg.Int("security.key_version", 1),
		AllowDev:   cfg.Bool("security.allow_dev_keys", true),
	})
	if err != nil {
		appLog.Error("ledger keys invalid", zap.String("error", err.Error()))
		panic(err)
	}
	if keys.IsDev() {
		appLog.Warn("ledger encryption is using built-in development keys; set LEDGER_MASTER_KEY/LEDGER_ID_KEY/LEDGER_MAC_KEY and LEDGER_ALLOW_DEV_KEYS=false in production")
	}

	shutdown, err := observability.InitTracer(context.Background(), "asset-service", cfg.String("tracing.endpoint", ""))
	if err != nil {
		appLog.Error("init tracer failed", zap.String("error", err.Error()))
	}
	defer func() { _ = shutdown(context.Background()) }()

	db, err := domain.OpenDBFromConfig(cfg, gormLogLevel(dev))
	if err != nil {
		panic(err)
	}
	if err := domain.Migrate(db); err != nil {
		panic(err)
	}
	if sqlDB, derr := db.DB(); derr == nil {
		observability.RegisterDBStats(sqlDB, "asset-service")
	}

	repo := repository.New(db)
	svc := service.New(repo, keys, service.Options{
		ReauthWindow:      durationOr(cfg.String("security.reauth_window", "5m"), 5*time.Minute),
		MaxItems:          cfg.Int("security.max_items", 2000),
		DeletionGraceDays: cfg.Int("security.deletion_grace_days", domain.LedgerDeletionGraceDays),
	})

	sweeper.New(svc, appLog, durationOr(cfg.String("security.sweeper_interval", "1h"), time.Hour)).
		Start(context.Background())

	h := handler.New(svc)

	server := httpx.NewServer(appLog, dev)
	engine := server.Engine()
	engine.Use(observability.MetricsMiddleware())
	engine.Use(observability.TracingMiddleware())
	observability.RegisterMetricsEndpoint(engine)
	engine.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok"})
	})
	h.RegisterRoutes(engine)

	addr := ":" + cfg.String("server.port", "8085")
	if err := server.Run(addr); err != nil {
		appLog.Error("server exit", zap.String("error", err.Error()))
	}
}

// masterKeys 读取主密钥版本表。
//
// 支持 security.ledger_master_key（当前版本）与
// security.ledger_master_key_previous（上一版本，仅解密），便于轮换。
func masterKeys(cfg *config.Loader) map[int]string {
	version := cfg.Int("security.key_version", 1)
	if version <= 0 {
		version = 1
	}
	out := map[int]string{}
	if cur := cfg.String("security.ledger_master_key", ""); cur != "" {
		out[version] = cur
	}
	if prev := cfg.String("security.ledger_master_key_previous", ""); prev != "" && version > 1 {
		out[version-1] = prev
	}
	return out
}

func durationOr(raw string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return def
}

func gormLogLevel(dev bool) gormlogger.LogLevel {
	if dev {
		return gormlogger.Info
	}
	return gormlogger.Warn
}
