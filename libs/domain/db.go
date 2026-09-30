package domain

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DBDialect 数据库方言，支持可替换关系型数据库。
type DBDialect string

const (
	DialectMySQL    DBDialect = "mysql"
	DialectPostgres DBDialect = "postgres"
	DialectSQLite   DBDialect = "sqlite"
)

// DefaultSlowQueryThreshold 未配置时的慢查询阈值。
const DefaultSlowQueryThreshold = 200 * time.Millisecond

// PoolConfig 数据库连接池配置（零值表示沿用驱动默认值）。
type PoolConfig struct {
	MaxOpenConns    int           // 最大打开连接数
	MaxIdleConns    int           // 最大空闲连接数
	ConnMaxLifetime time.Duration // 连接最长复用时间
	ConnMaxIdleTime time.Duration // 空闲连接最长存活时间
}

// dbOptions OpenDB 可选项。
type dbOptions struct {
	slowThreshold time.Duration
	pool          PoolConfig
	plugins       []gorm.Plugin
}

// DBOption 定制数据库初始化行为。
type DBOption func(*dbOptions)

// WithSlowQueryThreshold 设置慢查询阈值（超过即以 Warn 级记录 SQL 日志）。
func WithSlowQueryThreshold(d time.Duration) DBOption {
	return func(o *dbOptions) {
		if d > 0 {
			o.slowThreshold = d
		}
	}
}

// WithPool 设置连接池参数。
func WithPool(pool PoolConfig) DBOption {
	return func(o *dbOptions) { o.pool = pool }
}

// WithPlugins 注册 GORM 插件（如 OpenTelemetry 链路追踪插件）。
func WithPlugins(plugins ...gorm.Plugin) DBOption {
	return func(o *dbOptions) { o.plugins = append(o.plugins, plugins...) }
}

// OpenDB 根据方言与 DSN 建立数据库连接，实现关系型数据库可替换。
// 各服务通过配置指定 dialect 与 dsn，切换数据库无需改代码。
func OpenDB(dialect DBDialect, dsn string, logLevel logger.LogLevel, opts ...DBOption) (*gorm.DB, error) {
	o := &dbOptions{slowThreshold: DefaultSlowQueryThreshold}
	for _, opt := range opts {
		opt(o)
	}

	gcfg := logger.Config{
		SlowThreshold:             o.slowThreshold,
		LogLevel:                  logLevel,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	}
	cfg := &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gcfg),
	}

	var dialector gorm.Dialector
	switch dialect {
	case DialectMySQL:
		dialector = mysql.Open(dsn)
	case DialectPostgres:
		dialector = postgres.Open(dsn)
	case DialectSQLite:
		dialector = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database dialect: %s", dialect)
	}

	db, err := gorm.Open(dialector, cfg)
	if err != nil {
		return nil, err
	}

	// 注册插件（如 OTel 追踪：每条 SQL 一个 span，定位慢查询归属）。
	for _, p := range o.plugins {
		if err := db.Use(p); err != nil {
			return nil, fmt.Errorf("use gorm plugin %T: %w", p, err)
		}
	}

	// 连接池参数：未配置（零值）时保持驱动默认，避免影响现有部署。
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if o.pool.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(o.pool.MaxOpenConns)
	}
	if o.pool.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(o.pool.MaxIdleConns)
	}
	if o.pool.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(o.pool.ConnMaxLifetime)
	}
	if o.pool.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(o.pool.ConnMaxIdleTime)
	}
	return db, nil
}
