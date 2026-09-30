package domain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"peak/libs/config"
)

type testRow struct {
	ID  uint64 `gorm:"primaryKey"`
	Val string
}

func (testRow) TableName() string { return "test_rows" }

func openTestDB(t *testing.T, opts ...DBOption) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(DialectSQLite, dsn, logger.Silent, opts...)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&testRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestOpenDBSQLiteCRUD(t *testing.T) {
	db := openTestDB(t)
	if err := db.Create(&testRow{ID: 1, Val: "hello"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var row testRow
	if err := db.First(&row, 1).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.Val != "hello" {
		t.Fatalf("unexpected val: %s", row.Val)
	}
}

func TestOpenDBUnsupportedDialect(t *testing.T) {
	if _, err := OpenDB("oracle", "dsn", logger.Silent); err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
}

func TestOpenDBWithPlugins(t *testing.T) {
	prev := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prev)

	exp := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(exp)))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	db := openTestDB(t, WithPlugins(newGormTracingPlugin()))
	if err := db.Create(&testRow{ID: 2, Val: "traced"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var row testRow
	if err := db.First(&row, 2).Error; err != nil {
		t.Fatalf("query: %v", err)
	}

	spans := exp.Ended()
	if len(spans) == 0 {
		t.Fatal("expected gorm spans")
	}
	var foundInsert, foundQuery bool
	for _, s := range spans {
		name := s.Name()
		if len(name) > 6 && name[:6] == "INSERT" {
			foundInsert = true
		}
		if len(name) > 6 && name[:6] == "SELECT" {
			foundQuery = true
		}
	}
	if !foundInsert || !foundQuery {
		t.Fatalf("expected INSERT and SELECT spans, got %v", spanNames(spans))
	}
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, s := range spans {
		names = append(names, s.Name())
	}
	return names
}

func TestGormPluginRecoverContext(t *testing.T) {
	// 回调结束后 Statement.Context 应恢复为调用方原始上下文（可继续透传自定义值）。
	db := openTestDB(t, WithPlugins(newGormTracingPlugin()))
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	err := db.WithContext(ctx).Create(&testRow{ID: 3, Val: "ctx"}).Error
	if err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestOpenDBFromConfigSQLitePoolDefaults(t *testing.T) {
	cfg := mustLoad(t, `
database:
  dialect: sqlite
  dsn: "`+filepath.Join(t.TempDir(), "cfg.db")+`"
`)
	db, err := OpenDBFromConfig(cfg, logger.Silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	stats := sqlDB.Stats()
	// SQLite 未显式配置时应收敛为单连接防锁表。
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("expected sqlite default max open = 1, got %d", stats.MaxOpenConnections)
	}
}

func TestOpenDBFromConfigExplicitPool(t *testing.T) {
	cfg := mustLoad(t, `
database:
  dialect: sqlite
  dsn: "`+filepath.Join(t.TempDir(), "cfg.db")+`"
  max_open_conns: 4
  max_idle_conns: 2
  conn_max_lifetime: 30m
`)
	db, err := OpenDBFromConfig(cfg, logger.Silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, _ := db.DB()
	if got := sqlDB.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("expected max open = 4, got %d", got)
	}
}

func TestOpenDBSlowQueryThresholdOption(t *testing.T) {
	// 仅验证选项可安全传入（不触发慢查询日志）。
	db := openTestDB(t, WithSlowQueryThreshold(10*time.Millisecond))
	if err := db.Create(&testRow{ID: 4, Val: "slow-th"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestParseDurationOr(t *testing.T) {
	if got := parseDurationOr("bad", 5*time.Second); got != 5*time.Second {
		t.Fatalf("expected fallback, got %v", got)
	}
	if got := parseDurationOr("2h", 5*time.Second); got != 2*time.Hour {
		t.Fatalf("expected parsed, got %v", got)
	}
	if got := parseDurationOr("0s", 5*time.Second); got != 5*time.Second {
		t.Fatalf("expected fallback for non-positive, got %v", got)
	}
}

func mustLoad(t *testing.T, content string) *config.Loader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
