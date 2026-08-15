package domain

import (
	"fmt"

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

// OpenDB 根据方言与 DSN 建立数据库连接，实现关系型数据库可替换。
// 各服务通过配置指定 dialect 与 dsn，切换数据库无需改代码。
func OpenDB(dialect DBDialect, dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
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
	return db, nil
}
