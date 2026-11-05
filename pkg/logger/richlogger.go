package logger

import (
	"context"
	"fmt"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// WithContext returns a Logger whose entries carry ctx's trace and span IDs
// and the fields ContextWithFields stored in ctx, so the entries of one
// request or task can be found together.
func WithContext(ctx context.Context) Logger {
	return &richLogger{
		ctx: ctx,
	}
}

type richLogger struct {
	ctx        context.Context
	callerSkip int
	fields     []LogField
}

func (l *richLogger) Debug(v ...any) {
	if shallLog(DebugLevel) {
		msg, fields := splitLogArgs(v)
		l.debug(msg, fields...)
	}
}

func (l *richLogger) Debugf(format string, v ...any) {
	if shallLog(DebugLevel) {
		l.debug(fmt.Sprintf(format, v...))
	}
}

func (l *richLogger) Debugw(msg string, fields ...LogField) {
	if shallLog(DebugLevel) {
		l.debug(msg, fields...)
	}
}

func (l *richLogger) Error(v ...any) {
	if shallLog(ErrorLevel) {
		msg, fields := splitLogArgs(v)
		l.err(msg, fields...)
	}
}

func (l *richLogger) Errorf(format string, v ...any) {
	if shallLog(ErrorLevel) {
		l.err(fmt.Sprintf(format, v...))
	}
}

func (l *richLogger) Errorw(msg string, fields ...LogField) {
	if shallLog(ErrorLevel) {
		l.err(msg, fields...)
	}
}

func (l *richLogger) Info(v ...any) {
	if shallLog(InfoLevel) {
		msg, fields := splitLogArgs(v)
		l.info(msg, fields...)
	}
}

func (l *richLogger) Infof(format string, v ...any) {
	if shallLog(InfoLevel) {
		l.info(fmt.Sprintf(format, v...))
	}
}

func (l *richLogger) Infow(msg string, fields ...LogField) {
	if shallLog(InfoLevel) {
		l.info(msg, fields...)
	}
}

func (l *richLogger) Sloww(msg string, fields ...LogField) {
	if shallLog(ErrorLevel) {
		l.slow(msg, fields...)
	}
}

func (l *richLogger) WithCallerSkip(skip int) Logger {
	if skip <= 0 {
		return l
	}

	return &richLogger{
		ctx:        l.ctx,
		callerSkip: skip,
		fields:     l.fields,
	}
}

func (l *richLogger) WithDuration(duration time.Duration) Logger {
	fields := append(l.fields, Field(durationKey, reprLogDuration(duration)))

	return &richLogger{
		ctx:        l.ctx,
		callerSkip: l.callerSkip,
		fields:     fields,
	}
}

func (l *richLogger) buildFields(fields ...LogField) []LogField {
	fields = append(l.fields, fields...)
	fields = append(fields, Field(callerKey, getCaller(callerDepth+l.callerSkip)))

	if l.ctx == nil {
		return fields
	}

	spanContext := oteltrace.SpanContextFromContext(l.ctx)
	if spanContext.HasTraceID() {
		fields = append(fields, Field(traceKey, spanContext.TraceID().String()))
	}

	if spanContext.HasSpanID() {
		fields = append(fields, Field(spanKey, spanContext.SpanID().String()))
	}

	val := l.ctx.Value(fieldsContextKey)
	if val != nil {
		if arr, ok := val.([]LogField); ok {
			fields = append(fields, arr...)
		}
	}

	return redactFields(fields)
}

func (l *richLogger) debug(v any, fields ...LogField) {
	if shallLog(DebugLevel) {
		getWriter().Debug(redactValue(v), l.buildFields(fields...)...)
	}
}

func (l *richLogger) err(v any, fields ...LogField) {
	if shallLog(ErrorLevel) {
		getWriter().Error(redactValue(v), l.buildFields(fields...)...)
	}
}

func (l *richLogger) info(v any, fields ...LogField) {
	if shallLog(InfoLevel) {
		getWriter().Info(redactValue(v), l.buildFields(fields...)...)
	}
}

func (l *richLogger) slow(v any, fields ...LogField) {
	if shallLog(ErrorLevel) {
		getWriter().Slow(redactValue(v), l.buildFields(fields...)...)
	}
}
