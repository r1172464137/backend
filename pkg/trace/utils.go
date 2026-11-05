package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// TraceIDFromContext returns the trace id from ctx, or an empty string.
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanContextFromContext(ctx)
	if span.HasTraceID() {
		return span.TraceID().String()
	}
	return ""
}

// TracerFromContext returns the tracer of the span in ctx, or the global
// tracer when ctx has none, so a child span stays with its parent's
// provider.
func TracerFromContext(ctx context.Context) (tracer trace.Tracer) {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		tracer = span.TracerProvider().Tracer(TraceName)
	} else {
		tracer = otel.Tracer(TraceName)
	}
	return
}
