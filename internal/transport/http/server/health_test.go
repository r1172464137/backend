package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestHealthEndpoints(t *testing.T) {
	var readyErr error
	var sawDeadline bool
	engine := server.Default()
	registerHealth(engine, func(ctx context.Context) error {
		_, sawDeadline = ctx.Deadline()
		return readyErr
	})

	w := ut.PerformRequest(engine.Engine, http.MethodGet, "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("/healthz = %d", w.Code)
	}

	w = ut.PerformRequest(engine.Engine, http.MethodGet, "/readyz", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ready"`) || !sawDeadline {
		t.Fatalf("/readyz ready = %d %s deadline=%v", w.Code, w.Body.String(), sawDeadline)
	}

	readyErr = errors.New("database unavailable")
	w = ut.PerformRequest(engine.Engine, http.MethodGet, "/readyz", nil)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatalf("/readyz down = %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing no-store")
	}
}

// A dependency that hangs must not hold the probe beyond readinessTimeout.
func TestReadinessProbeIsBounded(t *testing.T) {
	engine := server.Default()
	registerHealth(engine, func(ctx context.Context) error {
		<-ctx.Done()
		return errors.New("database unavailable")
	})
	start := time.Now()
	w := ut.PerformRequest(engine.Engine, http.MethodGet, "/readyz", nil)
	if w.Code != http.StatusServiceUnavailable || time.Since(start) > readinessTimeout+time.Second {
		t.Fatalf("code=%d elapsed=%s", w.Code, time.Since(start))
	}
}

func TestReadinessWithoutChecker(t *testing.T) {
	engine := server.Default()
	registerHealth(engine, nil)
	if w := ut.PerformRequest(engine.Engine, http.MethodGet, "/readyz", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil checker = %d", w.Code)
	}
}
