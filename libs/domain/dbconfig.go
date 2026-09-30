package domain

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"peak/libs/config"
)

// OpenDBFromConfig 从统一配置（database.* 键）建立带追踪与连接池的数据库连接。
// 支持的配置项：
//
//	database.dialect / database.dsn        方言与 DSN（沿用原有键）
//	database.slow_query_threshold          慢查询日志阈值（duration 字符串，默认 200ms）
//	database.max_open_conns                最大打开连接数（默认 25；SQLite 默认 1 防锁表）
//	database.max_idle_conns                最大空闲连接数（默认 10）
//	database.conn_max_lifetime             连接最长复用时间（默认 1h）
//	database.conn_max_idle_time            空闲连接最长存活时间（默认 10m）
//
// 自动注册 GORM OpenTelemetry 插件：每条 SQL 产生一个 span（tracing 未启用时
// 为 no-op），用于在链路中定位"接口慢"是否卡在数据库与具体 SQL。
func OpenDBFromConfig(cfg *config.Loader, logLevel logger.LogLevel, extra ...DBOption) (*gorm.DB, error) {
	dialect := DBDialect(cfg.String("database.dialect", "mysql"))

	slow, err := time.ParseDuration(cfg.String("database.slow_query_threshold", "200ms"))
	if err != nil {
		slow = DefaultSlowQueryThreshold
	}
	pool := PoolConfig{
		MaxOpenConns:    cfg.Int("database.max_open_conns", 25),
		MaxIdleConns:    cfg.Int("database.max_idle_conns", 10),
		ConnMaxLifetime: parseDurationOr(cfg.String("database.conn_max_lifetime", "1h"), time.Hour),
		ConnMaxIdleTime: parseDurationOr(cfg.String("database.conn_max_idle_time", "10m"), 10*time.Minute),
	}
	// SQLite 是单写库，多连接反而容易触发 database is locked；
	// 显式配置过 max_open_conns 时尊重配置，否则收敛为 1。
	if dialect == DialectSQLite && cfg.Get("database.max_open_conns") == nil {
		pool.MaxOpenConns = 1
	}

	opts := append([]DBOption{
		WithSlowQueryThreshold(slow),
		WithPool(pool),
		WithPlugins(newGormTracingPlugin()),
	}, extra...)
	return OpenDB(dialect, cfg.String("database.dsn", ""), logLevel, opts...)
}

func parseDurationOr(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return def
}
