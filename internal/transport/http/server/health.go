package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// readinessTimeout bounds one readiness probe; a hung dependency must not hold
// the probe (or the orchestrator calling it) open.
const readinessTimeout = 3 * time.Second

// registerHealth adds unauthenticated probes:
//
//	GET /healthz  liveness: the process serves HTTP.
//	GET /readyz   readiness: database and Redis answer within the timeout.
//
// Responses carry only the name of a failing dependency, never addresses,
// credentials or driver messages.
func registerHealth(engine *server.Hertz, ready func(context.Context) error) {
	engine.GET("/healthz", func(c context.Context, ctx *app.RequestContext) {
		ctx.Header("Cache-Control", "no-store")
		ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	engine.GET("/readyz", func(c context.Context, ctx *app.RequestContext) {
		ctx.Header("Cache-Control", "no-store")
		if ready == nil {
			ctx.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "readiness not configured"})
			return
		}
		pctx, cancel := context.WithTimeout(c, readinessTimeout)
		defer cancel()
		if err := ready(pctx); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
}
