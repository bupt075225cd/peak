package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer 初始化 OpenTelemetry 链路追踪，返回关闭函数。
// 当 endpoint 为空时，返回 no-op，便于本地开发与测试。
// 无论是否启用，都注册 W3C TraceContext 传播器（跨服务 traceparent 透传依赖它）。
func InitTracer(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

// SpanAttribute 便于外部构造追踪属性。
func SpanAttribute(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// Tracer 返回业务侧自定义 span 使用的全局 tracer。
func Tracer() trace.Tracer {
	return otel.Tracer("peak/business")
}

// StartAISpan 为一次第三方 AI provider 调用创建 client span，
// 用于在链路中区分"慢在自家服务还是第三方 AI"。须与 FinishSpan 配对。
func StartAISpan(ctx context.Context, providerName, operation string) (context.Context, trace.Span) {
	return otel.Tracer("peak/observability").Start(ctx, "ai."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("ai.provider", providerName),
			attribute.String("ai.operation", operation),
		))
}

// FinishSpan 结束 span：失败时记录错误与错误状态。
func FinishSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
