package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"peak/libs/observability"
)

// 前端监控上报端点（POST /api/monitor/report）。
//
// 处理策略（公开接口，无鉴权，须防滥用）：
//   - 请求体上限 64KB（前端单批 ≤20 条事件，远小于该值）；
//   - 仅接受 JSON；解析失败静默丢弃（不返回 4xx 细节，避免探测）；
//   - 每条事件：
//       1. 计入 Prometheus 计数器 frontend_report_total{type,name}
//          —— Web Vitals / PV / 错误在 Grafana 出趋势图与告警；
//       2. 以 zap JSON 落 stdout（含 trace_id）——Alloy 采集入 Loki，
//          可按 trace_id 与后端链路互查，错误事件附 message/stack 供定位。
//   - 事件类型/名称白名单校验，防脏数据打爆标签基数（cardinality）。

const monitorMaxBodyBytes = 64 << 10 // 64KB

// monitorAllowedTypes 允许的事件大类（与前端 SDK types.ts 对齐）。
var monitorAllowedTypes = map[string]bool{
	"error": true, "webvital": true, "pv": true, "behavior": true,
}

// monitorEvent 单条前端事件（宽松解析，字段缺失不报错）。
type monitorEvent struct {
	Type    string         `json:"type"`
	Name    string         `json:"name"`
	Data    map[string]any `json:"data"`
	TS      int64          `json:"ts"`
	Path    string         `json:"path"`
	TraceID string         `json:"trace_id"`
}

type monitorReportRequest struct {
	Events []monitorEvent `json:"events"`
}

// sanitizeEvent 校验并收敛事件字段：类型白名单、名称长度截断、
// path/trace_id 规范化。返回 ok=false 表示丢弃。
func sanitizeEvent(e *monitorEvent) bool {
	if !monitorAllowedTypes[e.Type] {
		return false
	}
	if len(e.Name) == 0 || len(e.Name) > 64 {
		return false
	}
	if len(e.Path) > 256 {
		e.Path = e.Path[:256]
	}
	e.Path = strings.TrimSpace(e.Path)
	// trace_id 只允许 hex（W3C 32 位）或短字符串，防日志注入。
	e.TraceID = sanitizeTraceID(e.TraceID)
	return true
}

func sanitizeTraceID(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 64 {
		s = s[:64]
	}
	for _, r := range s {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || r == '-'
		if !ok {
			return ""
		}
	}
	return s
}

// registerMonitorReport 注册上报端点。
func registerMonitorReport(engine *gin.Engine, log *zap.Logger) {
	engine.POST("/api/monitor/report", func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, monitorMaxBodyBytes+1))
		if err != nil || len(body) > monitorMaxBodyBytes {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		var req monitorReportRequest
		if err := json.Unmarshal(body, &req); err != nil {
			// 静默丢弃非法请求：公开端点不给探测者任何信息。
			c.Status(http.StatusNoContent)
			return
		}

		traceID := c.GetString("trace_id")
		for i := range req.Events {
			e := &req.Events[i]
			if !sanitizeEvent(e) {
				continue
			}
			// 事件自带 trace_id（页面级）缺失时，用网关请求级 trace_id 兜底。
			tid := e.TraceID
			if tid == "" {
				tid = traceID
			}
			observability.FrontendReportTotal.WithLabelValues(e.Type, e.Name).Inc()
			log.Info("frontend event",
				zap.String("event_type", e.Type),
				zap.String("event_name", e.Name),
				zap.Any("data", e.Data),
				zap.Int64("event_ts", e.TS),
				zap.String("path", e.Path),
				zap.String("client_trace_id", tid),
			)
		}
		c.Status(http.StatusNoContent)
	})
}
