// question-service 是题目/错题核心领域服务，负责错题、题目、分类的 CRUD 与手动修正。
package main

import (
	"context"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	gormlogger "gorm.io/gorm/logger"

	"peak/libs/config"
	"peak/libs/domain"
	httpx "peak/libs/http"
	"peak/libs/logger"
	"peak/libs/observability"
	"peak/libs/storage"

	"peak/apps/question-service/internal/export"
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

	// 初始化存储（与 recognition-service 共用同一存储与桶：负责 committed/ 正式区
	// 的读取与 transient/ -> committed/ 的提交拷贝）。
	store, err := newStorage(cfg)
	if err != nil {
		panic(err)
	}

	// 组装依赖：repository -> service -> handler。
	repos := repository.NewGormRepositories(db)

	// 导出能力在启动阶段初始化：字体加载等问题会立即暴露，而不是等用户点导出。
	exporter, err := export.NewDefaultWithFetcher(exportConfig(cfg), export.NewStorageFetcher(store))
	if err != nil {
		panic(err)
	}
	svc := service.New(repos, exporter)
	h := handler.New(svc, store)

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

// exportConfig 从配置构建导出配置，未配置项沿用默认值。
func exportConfig(cfg *config.Loader) export.Config {
	ec := export.DefaultConfig()
	ec.FontPath = cfg.String("export.font_path", ec.FontPath)
	ec.MaxItems = cfg.Int("export.max_items", ec.MaxItems)
	ec.MaxImageWidth = cfg.Int("export.max_image_width", ec.MaxImageWidth)
	ec.MergeStemLineBreaks = cfg.Bool("export.merge_stem_line_breaks", ec.MergeStemLineBreaks)

	if raw := cfg.String("export.fetch_timeout", ""); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			ec.FetchTimeout = d
		}
	}
	return ec
}

// newStorage 按配置创建存储后端：storage.type 为 "s3" 时返回 S3 兼容存储，
// 否则（含空值）回退本地磁盘（storage.root）。与 recognition-service 的
// 配置口径一致，两端必须指向同一存储/桶。
func newStorage(cfg *config.Loader) (storage.FileStorage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.String("storage.type", "local"))) {
	case "s3":
		return storage.NewS3Storage(storage.Config{
			Endpoint:  cfg.String("storage.s3.endpoint", ""),
			Region:    cfg.String("storage.s3.region", "us-east-1"),
			AccessKey: cfg.String("storage.s3.access_key", ""),
			SecretKey: cfg.String("storage.s3.secret_key", ""),
			Bucket:    cfg.String("storage.s3.bucket", "peak"),
			UseSSL:    cfg.Bool("storage.s3.use_ssl", false),
			PathStyle: cfg.Bool("storage.s3.path_style", false),
		})
	default:
		return storage.NewLocalStorage(cfg.String("storage.root", "./data"))
	}
}
