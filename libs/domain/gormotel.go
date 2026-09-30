package domain

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// gormTracingPlugin 轻量 GORM OpenTelemetry 插件：每条 SQL 产生一个 client
// span（表名、参数化 SQL、影响行数、错误），用于在链路中定位慢查询。
// 参考官方 gorm.io/plugin/opentelemetry/tracing 实现，但不引入 ClickHouse
// 等无关驱动依赖，并固定使用本项目 otel 版本。tracing 未启用时 span 为
// noop，开销可忽略。
type gormTracingPlugin struct {
	tracer trace.Tracer
}

func newGormTracingPlugin() gorm.Plugin {
	return &gormTracingPlugin{tracer: otel.GetTracerProvider().Tracer("peak/gorm")}
}

func (p *gormTracingPlugin) Name() string { return "peak:otel-tracing" }

type gormHookFunc func(tx *gorm.DB)

type gormRegister interface {
	Register(name string, fn func(*gorm.DB)) error
}

func (p *gormTracingPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()
	hooks := []struct {
		callback gormRegister
		hook     gormHookFunc
		name     string
	}{
		{cb.Create().Before("gorm:create"), p.before("gorm.Create"), "before:create"},
		{cb.Create().After("gorm:create"), p.after(), "after:create"},
		{cb.Query().Before("gorm:query"), p.before("gorm.Query"), "before:select"},
		{cb.Query().After("gorm:query"), p.after(), "after:select"},
		{cb.Delete().Before("gorm:delete"), p.before("gorm.Delete"), "before:delete"},
		{cb.Delete().After("gorm:delete"), p.after(), "after:delete"},
		{cb.Update().Before("gorm:update"), p.before("gorm.Update"), "before:update"},
		{cb.Update().After("gorm:update"), p.after(), "after:update"},
		{cb.Row().Before("gorm:row"), p.before("gorm.Row"), "before:row"},
		{cb.Row().After("gorm:row"), p.after(), "after:row"},
		{cb.Raw().Before("gorm:raw"), p.before("gorm.Raw"), "before:raw"},
		{cb.Raw().After("gorm:raw"), p.after(), "after:raw"},
	}
	var firstErr error
	for _, h := range hooks {
		if err := h.callback.Register("otel:"+h.name, h.hook); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("register callback %s: %w", h.name, err)
		}
	}
	return firstErr
}

// ctxWrapper 保存 span 上下文，回调结束后恢复调用方的原始上下文。
type ctxWrapper struct {
	context.Context
	parent context.Context
}

func (p *gormTracingPlugin) before(op string) gormHookFunc {
	return func(tx *gorm.DB) {
		parentCtx := tx.Statement.Context
		ctx, _ := p.tracer.Start(parentCtx, op, trace.WithSpanKind(trace.SpanKindClient))
		tx.Statement.Context = ctxWrapper{ctx, parentCtx}
	}
}

func (p *gormTracingPlugin) after() gormHookFunc {
	return func(tx *gorm.DB) {
		defer func() {
			if c, ok := tx.Statement.Context.(ctxWrapper); ok {
				tx.Statement.Context = c.parent
			}
		}()
		span := trace.SpanFromContext(tx.Statement.Context)
		if !span.IsRecording() {
			return
		}
		defer span.End()

		if tx.Statement.Table != "" {
			span.SetName(opName(tx) + " " + tx.Statement.Table)
			span.SetAttributes(attribute.String("db.table", tx.Statement.Table))
		}
		// 参数化 SQL（Explain 展开，不含用户敏感参数明文之外的信息——与
		// gorm logger 行为一致；生产 DB 日志本身也打印该形态）。
		if stmt := tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...); stmt != "" {
			span.SetAttributes(attribute.String("db.statement", truncateSQL(stmt, 1024)))
		}
		if tx.Statement.RowsAffected != -1 {
			span.SetAttributes(attribute.Int64("db.rows_affected", tx.Statement.RowsAffected))
		}
		switch tx.Error {
		case nil, gorm.ErrRecordNotFound, driver.ErrSkip, sql.ErrNoRows, io.EOF:
			// 常规返回（含"未找到记录"），不算错误。
		default:
			span.RecordError(tx.Error)
			span.SetStatus(codes.Error, tx.Error.Error())
		}
	}
}

// opName 取 SQL 首个单词（SELECT/INSERT/…）作为操作名，空 SQL 回退 gorm。
func opName(tx *gorm.DB) string {
	s := tx.Statement.SQL.String()
	if s == "" {
		return "gorm"
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return s[:i]
		}
	}
	return s
}

func truncateSQL(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
