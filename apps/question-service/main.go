// question-service 是题目/错题核心领域服务，负责错题、题目、分类的 CRUD 与手动修正。
package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
		observability.RegisterDBStats(sqlDB, "question-service")
	}

	// 初始化存储：question-service 使用专属桶/目录存放 committed/ 正式区；
	// recognition-service 的桶作为源，提交拷贝走对象存储的服务端 CopyObject。
	store, copier, err := newStorage(cfg)
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
	h := handler.New(svc, store, copier, db, cfg.String("files.sign_secret", ""))

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

// newStorage 按配置创建存储后端与跨桶拷贝器：storage.type 为 "s3" 时返回
// S3 兼容存储，否则（含空值）回退本地磁盘（storage.root）。
//
// 目标存储使用本服务专属桶（storage.s3.bucket，存放 committed/ 正式区）；
// 源存储指向 recognition-service 的桶（storage.source.s3.bucket，transient/
// 临时区）。S3 后端的提交拷贝通过服务端 CopyObject 跨桶完成（Copier 使用
// 目标存储客户端 + 源桶名，源桶需与目标桶在同一 Endpoint 下且凭证可读）；
// 本地后端回退为跨目录复制，源目录由 storage.source.root 指定。
func newStorage(cfg *config.Loader) (storage.FileStorage, *storage.Copier, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.String("storage.type", "local"))) {
	case "s3":
		store, err := storage.NewS3Storage(storage.Config{
			Endpoint:  cfg.String("storage.s3.endpoint", ""),
			Region:    cfg.String("storage.s3.region", "us-east-1"),
			AccessKey: cfg.String("storage.s3.access_key", ""),
			SecretKey: cfg.String("storage.s3.secret_key", ""),
			Bucket:    cfg.String("storage.s3.bucket", "peak-question"),
			UseSSL:    cfg.Bool("storage.s3.use_ssl", false),
			PathStyle: cfg.Bool("storage.s3.path_style", false),
		})
		if err != nil {
			return nil, nil, err
		}
		// 源桶与目标桶共用同一 Endpoint/凭证（不同桶名）；源存储仅作为
		// Copier 的回退路径（S3 目标存储实现 ObjectCopier，一般不会用到）。
		src, err := storage.NewS3Storage(storage.Config{
			Endpoint:  cfg.String("storage.s3.endpoint", ""),
			Region:    cfg.String("storage.s3.region", "us-east-1"),
			AccessKey: cfg.String("storage.s3.access_key", ""),
			SecretKey: cfg.String("storage.s3.secret_key", ""),
			Bucket:    cfg.String("storage.source.s3.bucket", "peak-recognition"),
			UseSSL:    cfg.Bool("storage.s3.use_ssl", false),
			PathStyle: cfg.Bool("storage.s3.path_style", false),
		})
		if err != nil {
			return nil, nil, err
		}
		srcBucket := cfg.String("storage.source.s3.bucket", "peak-recognition")
		return store, storage.NewCopier(store, src, srcBucket), nil
	default:
		store, err := storage.NewLocalStorage(cfg.String("storage.root", "./data"))
		if err != nil {
			return nil, nil, err
		}
		src, err := storage.NewLocalStorage(cfg.String("storage.source.root", "./data"))
		if err != nil {
			return nil, nil, err
		}
		return store, storage.NewCopier(store, src, ""), nil
	}
}
