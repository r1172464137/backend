package trace

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceIDFromContext(t *testing.T) {
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	if got := TraceIDFromContext(ctx); got != spanCtx.TraceID().String() {
		t.Fatalf("TraceIDFromContext() = %q, want %q", got, spanCtx.TraceID())
	}
}

func TestTraceIDFromContextWithoutSpan(t *testing.T) {
	if got := TraceIDFromContext(context.Background()); got != "" {
		t.Fatalf("TraceIDFromContext() = %q, want empty", got)
	}
}
