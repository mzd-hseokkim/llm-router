package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "gateway"

// Tracer returns the gateway's named OTel tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// StartSpan starts a new child span with the given name.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, name,
		trace.WithAttributes(attrs...),
	)
	return ctx, span
}

// RecordError marks a span as failed and records the error.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// ExtractTraceContext reads W3C traceparent/tracestate from an inbound HTTP request
// and returns a context that is a child of the upstream trace (if present).
func ExtractTraceContext(r *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
}

// InjectTraceContext writes the current trace context into outbound HTTP request headers.
func InjectTraceContext(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// TraceIDFromContext returns the current trace ID as a hex string, or "" if none.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// SpanIDFromContext returns the current span ID as a hex string, or "" if none.
func SpanIDFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		return sc.SpanID().String()
	}
	return ""
}
