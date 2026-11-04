// Package observability 提供 Prometheus 指标与 OpenTelemetry 链路追踪能力。
package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal 请求总数（按 method、path、status 维度）。
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	// HTTPRequestDuration 请求耗时直方图。
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// RecognitionTaskDuration 识别任务端到端耗时直方图（provider、最终状态）。
	RecognitionTaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "recognition_task_duration_seconds",
		Help: "Recognition task end-to-end latency in seconds.",
		Buckets: []float64{.5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600},
	}, []string{"provider", "status"})

	// RecognitionTaskTotal 识别任务完成总数（按 provider、最终状态 success/failed）。
	RecognitionTaskTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "recognition_task_total",
		Help: "Total number of finished recognition tasks by outcome.",
	}, []string{"provider", "status"})

	// AICallDuration 第三方 AI provider 单次调用耗时直方图。
	AICallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "ai_call_duration_seconds",
		Help: "Third-party AI provider call latency in seconds.",
		// AI 调用普遍在秒级（VLM 常见 5~60s），桶向大值偏移。
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300},
	}, []string{"provider", "operation", "status"})

	// AICallTotal 第三方 AI provider 调用总数（按 provider、operation、status）。
	AICallTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_call_total",
		Help: "Total number of third-party AI provider calls.",
	}, []string{"provider", "operation", "status"})

	// MistakeOpsTotal 错题关键操作总数（operation: create/export/...，status: ok/error）。
	MistakeOpsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mistake_ops_total",
		Help: "Total number of mistake business operations.",
	}, []string{"operation", "status"})

	// FrontendReportTotal 前端监控上报事件总数（type: error/webvital/pv/behavior，name: 具体事件名）。
	FrontendReportTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "frontend_report_total",
		Help: "Total number of frontend monitoring events received.",
	}, []string{"type", "name"})

	// GeometryRedrawDuration 几何重绘阶段整体耗时直方图（provider、outcome）。
	GeometryRedrawDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "geometry_redraw_duration_seconds",
		Help: "Geometry redraw stage total latency in seconds.",
		// 重绘含多轮 VLM 提取，通常 30~300s，桶向大值偏移。
		Buckets: []float64{5, 10, 15, 30, 60, 120, 180, 240, 300, 360, 600},
	}, []string{"provider", "outcome"})

	// GeometryRedrawOutcome 几何重绘结果计数（outcome: ok/empty/timeout/failed）。
	GeometryRedrawOutcome = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "geometry_redraw_total",
		Help: "Total geometry redraw results by outcome.",
	}, []string{"provider", "outcome"})

	// GeometryRedrawAttempts 每次重绘实际执行的"提取→渲染"轮数直方图。
	GeometryRedrawAttempts = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "geometry_redraw_attempts",
		Help:    "Geometry redraw extraction-render rounds per task.",
		Buckets: []float64{1, 2, 3, 4, 5, 6, 8, 10},
	}, []string{"provider"})

	// GeometryRedrawIssueCount 重绘校验问题计数（category: structural/semantic）。
	GeometryRedrawIssueCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "geometry_redraw_issues_total",
		Help: "Geometry redraw validation issues by category (structural or semantic).",
	}, []string{"provider", "category"})
)

// ObserveAICall 记录一次 AI provider 调用的耗时与结果（成功记 ok，失败记 error）。
func ObserveAICall(providerName, operation string, err error, start time.Time) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	d := time.Since(start).Seconds()
	AICallDuration.WithLabelValues(providerName, operation, status).Observe(d)
	AICallTotal.WithLabelValues(providerName, operation, status).Inc()
}

// ObserveMistakeOp 记录一次错题业务操作的结果。
func ObserveMistakeOp(operation string, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	MistakeOpsTotal.WithLabelValues(operation, status).Inc()
}

// ObserveGeometryRedraw 记录一次几何重绘的结果：耗时（按 outcome 分桶）、实际轮数、
// 结构/语义问题数。用于观察重绘耗时与准确率，以及结构/语义分流回喂修正的效果。
func ObserveGeometryRedraw(provider, outcome string, attempts, structIssues, semanticIssues int, start time.Time) {
	GeometryRedrawDuration.WithLabelValues(provider, outcome).Observe(time.Since(start).Seconds())
	GeometryRedrawOutcome.WithLabelValues(provider, outcome).Inc()
	if attempts > 0 {
		GeometryRedrawAttempts.WithLabelValues(provider).Observe(float64(attempts))
	}
	if structIssues > 0 {
		GeometryRedrawIssueCount.WithLabelValues(provider, "structural").Add(float64(structIssues))
	}
	if semanticIssues > 0 {
		GeometryRedrawIssueCount.WithLabelValues(provider, "semantic").Add(float64(semanticIssues))
	}
}
