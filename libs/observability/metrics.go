// Package observability 提供 Prometheus 指标与 OpenTelemetry 链路追踪能力。
package observability

import (
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

	// RecognitionTaskDuration 识别任务耗时直方图。
	RecognitionTaskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "recognition_task_duration_seconds",
		Help:    "Recognition task latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "status"})
)
