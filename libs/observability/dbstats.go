package observability

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// RegisterDBStats 注册数据库连接池指标采集器（基于 sql.DBStats）：
// db_connections_open/in_use/idle、db_wait_count、db_wait_duration 等，
// 用于告警连接池饱和与等待堆积（很多"应用卡死"其实是连接池耗尽）。
// 重复注册（如测试场景）安全忽略。
func RegisterDBStats(db *sql.DB, dbName string) {
	if db == nil {
		return
	}
	_ = prometheus.Register(collectors.NewDBStatsCollector(db, dbName))
}
