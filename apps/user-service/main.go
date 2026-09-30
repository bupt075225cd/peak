// user-service 是用户服务，负责手机验证码登录（自动注册）、JWT 签发与用户信息。
package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/apps/user-service/internal/code"
	"peak/apps/user-service/internal/handler"
	"peak/apps/user-service/internal/mail"
	"peak/apps/user-service/internal/repository"
	"peak/apps/user-service/internal/service"
	"peak/apps/user-service/internal/sms"
	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
)

func main() {
	cfgPath := os.Getenv("USER_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		panic(err)
	}

	appLog := logger.New(cfg.Bool("log.development", true))
	zapLog := appLog.Logger // 内嵌 *zap.Logger，供 mock 短信发送器记录日志。

	shutdown, err := observability.InitTracer(context.Background(), "user-service", cfg.String("tracing.endpoint", ""))
	if err != nil {
		appLog.Error("init tracer failed", zap.String("error", err.Error()))
	}
	defer func() { _ = shutdown(context.Background()) }()

	// 初始化数据库。
	dialect := cfg.String("database.dialect", "mysql")
	dsn := cfg.String("database.dsn", "")
	if dialect == "sqlite" {
		// SQLite 需确保库文件目录存在。
		if dir := filepath.Dir(dsn); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}
	}
	db, err := domain.OpenDBFromConfig(cfg, gormLogLevel(cfg.Bool("log.development", true)))
	if err != nil {
		panic(err)
	}
	if err := domain.Migrate(db); err != nil {
		panic(err)
	}
	// 暴露连接池指标（连接数/等待数/等待时长），用于告警连接池饱和。
	if sqlDB, derr := db.DB(); derr == nil {
		observability.RegisterDBStats(sqlDB, "user-service")
	}

	// 组装认证依赖：repository -> code store -> sms/mail sender -> service -> handler。
	repos := repository.NewUserRepository(db)
	codes := code.NewStore(code.Config{})
	sender := newSender(cfg, zapLog)
	mailer := newMailSender(cfg, zapLog)

	tokenTTL, err := time.ParseDuration(cfg.String("auth.token_ttl", "168h"))
	if err != nil {
		panic(err)
	}
	svc := service.New(repos, codes, sender, mailer, service.Config{
		JWTSecret: cfg.String("auth.jwt_secret", ""),
		TokenTTL:  tokenTTL,
		// 超级验证码双开关：配置显式开启 且 仅开发模式生效；生产强制禁用。
		MasterCode: cfg.Bool("auth.master_code", false) && cfg.Bool("log.development", true),
		Debug:      cfg.Bool("log.development", true),
	})
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

	addr := ":" + cfg.String("server.port", "8083")
	if err := server.Run(addr); err != nil {
		appLog.Error("server exit", zap.String("error", err.Error()))
	}
}

// newSender 按配置创建短信发送通道（当前仅 mock，真实短信下一迭代接入）。
func newSender(cfg *config.Loader, log *zap.Logger) sms.Sender {
	switch cfg.String("sms.provider", "mock") {
	case "mock":
		return sms.NewMockSender(log)
	default:
		return sms.NewMockSender(log)
	}
}

// newMailSender 按配置创建邮件发送通道：mock（开发，写日志）或 smtp（生产）。
func newMailSender(cfg *config.Loader, log *zap.Logger) mail.Sender {
	switch cfg.String("mail.provider", "mock") {
	case "smtp":
		return mail.NewSMTPSender(mail.SMTPConfig{
			Host:     cfg.String("mail.smtp.host", ""),
			Username: cfg.String("mail.smtp.username", ""),
			Password: cfg.String("mail.smtp.password", ""),
			From:     cfg.String("mail.smtp.from", ""),
		})
	default:
		return mail.NewMockSender(log)
	}
}

func gormLogLevel(dev bool) gormlogger.LogLevel {
	if dev {
		return gormlogger.Info
	}
	return gormlogger.Warn
}
