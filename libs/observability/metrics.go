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
