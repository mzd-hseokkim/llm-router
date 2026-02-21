package telemetry

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitOTel initialises the OpenTelemetry SDK.
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, a no-op tracer is used (zero cost).
// Returns a shutdown function that must be called on program exit.
func InitOTel(ctx context.Context, serviceName, version string, logger *slog.Logger) func(context.Context) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No-op: use the default global no-op tracer.
		logger.Debug("otel: OTEL_EXPORTER_OTLP_ENDPOINT not set; tracing disabled")
		return func(context.Context) error { return nil }
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		logger.Warn("otel: failed to create resource", "error", err)
		return func(context.Context) error { return nil }
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		logger.Warn("otel: failed to create OTLP exporter", "error", err)
		return func(context.Context) error { return nil }
	}

	// Use parent-based sampler: always sample errors, 10% of normal requests.
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(0.1),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("otel: tracing enabled", "endpoint", endpoint)

	return tp.Shutdown
}
