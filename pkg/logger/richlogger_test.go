package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const testlog = "Stay hungry, stay foolish."

func TestTraceLog(t *testing.T) {
	setRichLoggerTestLevel(t, InfoLevel)
	w, ctx := captureTraced(t)

	WithContext(ctx).Info(testlog)
	validate(t, w.String(), true, true)
}

func TestTraceDebug(t *testing.T) {
	w, ctx := captureTraced(t)

	l := WithContext(ctx)
	setRichLoggerTestLevel(t, DebugLevel)
	l.WithDuration(time.Second).Debug(testlog)
	assert.True(t, strings.Contains(w.String(), traceKey))
	assert.True(t, strings.Contains(w.String(), spanKey))
	w.Reset()
	l.WithDuration(time.Second).Debugf(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Debugw(testlog, Field("foo", "bar"))
	validate(t, w.String(), true, true)
	assert.True(t, strings.Contains(w.String(), "foo"), w.String())
	assert.True(t, strings.Contains(w.String(), "bar"), w.String())
}

func TestTraceError(t *testing.T) {
	w, ctx := captureTraced(t)

	l := WithContext(ctx)
	setRichLoggerTestLevel(t, ErrorLevel)
	l.WithDuration(time.Second).Error(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Errorf(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Errorw(testlog, Field("basket", "ball"))
	validate(t, w.String(), true, true)
	assert.True(t, strings.Contains(w.String(), "basket"), w.String())
	assert.True(t, strings.Contains(w.String(), "ball"), w.String())
}

func TestTraceInfo(t *testing.T) {
	w, ctx := captureTraced(t)

	setRichLoggerTestLevel(t, InfoLevel)
	l := WithContext(ctx)
	l.WithDuration(time.Second).Info(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Infof(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Infow(testlog, Field("basket", "ball"))
	validate(t, w.String(), true, true)
	assert.True(t, strings.Contains(w.String(), "basket"), w.String())
	assert.True(t, strings.Contains(w.String(), "ball"), w.String())
}

func TestTraceInfoConsole(t *testing.T) {
	old := atomic.SwapUint32(&encoding, jsonEncodingType)
	defer atomic.StoreUint32(&encoding, old)

	w, ctx := captureTraced(t)

	l := WithContext(ctx)
	setRichLoggerTestLevel(t, InfoLevel)
	l.WithDuration(time.Second).Info(testlog)
	validate(t, w.String(), true, true)
	w.Reset()
	l.WithDuration(time.Second).Infof(testlog)
	validate(t, w.String(), true, true)
}

func TestTraceSlow(t *testing.T) {
	w, ctx := captureTraced(t)

	l := WithContext(ctx)
	setRichLoggerTestLevel(t, InfoLevel)
	l.WithDuration(time.Second).Sloww(testlog, Field("basket", "ball"))
	validate(t, w.String(), true, true)
	assert.True(t, strings.Contains(w.String(), "basket"), w.String())
	assert.True(t, strings.Contains(w.String(), "ball"), w.String())
}

func TestTraceWithoutContext(t *testing.T) {
	w := new(mockWriter)
	old := writer.Swap(w)
	writer.lock.RLock()
	defer func() {
		writer.lock.RUnlock()
		writer.Store(old)
	}()

	l := WithContext(context.Background())
	setRichLoggerTestLevel(t, InfoLevel)
	l.WithDuration(time.Second).Info(testlog)
	validate(t, w.String(), false, false)
	w.Reset()
	l.WithDuration(time.Second).Infof(testlog)
	validate(t, w.String(), false, false)
}

func TestLogWithFields(t *testing.T) {
	w := new(mockWriter)
	old := writer.Swap(w)
	writer.lock.RLock()
	defer func() {
		writer.lock.RUnlock()
		writer.Store(old)
	}()

	ctx := ContextWithFields(context.Background(), Field("foo", "bar"))
	l := WithContext(ctx)
	setRichLoggerTestLevel(t, InfoLevel)
	l.Infow(testlog)

	var val mockValue
	assert.Nil(t, json.Unmarshal([]byte(w.String()), &val))
	assert.Equal(t, "bar", val.Foo)
}

func TestLogWithCallerSkip(t *testing.T) {
	setRichLoggerTestLevel(t, InfoLevel)

	w := new(mockWriter)
	old := writer.Swap(w)
	writer.lock.RLock()
	defer func() {
		writer.lock.RUnlock()
		writer.Store(old)
	}()

	l := new(richLogger).WithCallerSkip(1).WithCallerSkip(0)
	p := func(v string) {
		l.Infow(v)
	}

	file, line := getFileLine()
	p(testlog)
	assert.True(t, w.Contains(fmt.Sprintf("%s:%d", file, line+1)))

	w.Reset()
	l = new(richLogger).WithCallerSkip(0).WithCallerSkip(1)
	file, line = getFileLine()
	p(testlog)
	assert.True(t, w.Contains(fmt.Sprintf("%s:%d", file, line+1)))
}

func TestLogWithCallerSkipCopy(t *testing.T) {
	log1 := new(richLogger).WithCallerSkip(2)
	log2 := log1.WithCallerSkip(3)
	log3 := log2.WithCallerSkip(-1)
	assert.Equal(t, 2, log1.(*richLogger).callerSkip)
	assert.Equal(t, 3, log2.(*richLogger).callerSkip)
	assert.Equal(t, 3, log3.(*richLogger).callerSkip)
}

func TestLogWithDurationCopy(t *testing.T) {
	setRichLoggerTestLevel(t, InfoLevel)

	log1 := WithContext(context.Background())
	log2 := log1.WithDuration(time.Second)
	assert.Empty(t, log1.(*richLogger).fields)
	assert.Equal(t, 1, len(log2.(*richLogger).fields))

	var w mockWriter
	old := writer.Swap(&w)
	defer writer.Store(old)
	log2.Info("hello")
	assert.Contains(t, w.String(), `"duration":"1000.0ms"`)
}

func setRichLoggerTestLevel(t *testing.T, level uint32) {
	oldLevel := atomic.SwapUint32(&logLevel, level)
	t.Cleanup(func() {
		atomic.StoreUint32(&logLevel, oldLevel)
	})
}

func validate(t *testing.T, body string, expectedTrace, expectedSpan bool) {
	var val mockValue
	dec := json.NewDecoder(strings.NewReader(body))

	for {
		var doc mockValue
		err := dec.Decode(&doc)
		if err == io.EOF {
			// all done
			break
		}
		if err != nil {
			continue
		}

		val = doc
	}

	assert.Equal(t, expectedTrace, len(val.Trace) > 0, body)
	assert.Equal(t, expectedSpan, len(val.Span) > 0, body)
}

type mockValue struct {
	Trace   string `json:"trace"`
	Span    string `json:"span"`
	Foo     string `json:"foo"`
	Content any    `json:"content"`
}

// captureTraced captures the log output in a mock writer and returns a
// context carrying a sampled span; the writer, the tracer provider and the
// span are restored and ended when the test ends.
func captureTraced(t *testing.T) (*mockWriter, context.Context) {
	t.Helper()
	w := new(mockWriter)
	old := writer.Swap(w)
	writer.lock.RLock()
	t.Cleanup(func() {
		writer.lock.RUnlock()
		writer.Store(old)
	})

	otp := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(otp) })

	ctx, span := tp.Tracer("trace-id").Start(context.Background(), "span-id")
	t.Cleanup(func() { span.End() })
	return w, ctx
}
