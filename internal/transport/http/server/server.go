package httpserver

import (
	"context"
	"crypto/tls"
	"os"
	"sync/atomic"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	billingHTTP "github.com/perfect-panel/server/internal/module/billing/transport/http"
	"github.com/perfect-panel/server/internal/module/notification"
	notificationHTTP "github.com/perfect-panel/server/internal/module/notification/transport/http"
	"github.com/perfect-panel/server/internal/transport/http/middleware"
	"github.com/perfect-panel/server/internal/transport/http/routes"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
)

type Server struct {
	h        *server.Hertz
	stopping atomic.Bool
}

type Dependencies struct {
	Routes           routes.Dependencies
	Notification     notification.Service
	TelegramBotToken func() string
	RequestMetadata  requestmeta.Enricher
	// Readiness backs GET /readyz; nil reports "not configured" (503).
	Readiness func(context.Context) error
}

func New(deps Dependencies, addr string, tlsConfig *tls.Config) *Server {
	opts := []config.Option{
		server.WithHostPorts(addr),
		server.WithDisablePrintRoute(true),
		server.WithMaxRequestBodySize(4 * 1024 * 1024),
	}
	if tlsConfig != nil {
		opts = append(opts, server.WithTLS(tlsConfig))
	}

	return newServer(deps, opts)
}

func newServer(deps Dependencies, opts []config.Option) *Server {
	engine := server.Default(opts...)
	engine.Use(middleware.TraceMiddleware(), middleware.LoggerMiddleware(deps.RequestMetadata), middleware.CorsMiddleware)

	registerHealth(engine, deps.Readiness)
	routes.RegisterHandlers(engine, deps.Routes)
	notificationHTTP.RegisterTelegramHandlers(engine, deps.Notification, deps.TelegramBotToken)
	billingHTTP.RegisterNotifyHandlers(engine, deps.Routes.Store, deps.Routes.Billing)

	return &Server{h: engine}
}

// Start serves until Shutdown. A listener that fails (bind error, or the
// netpoll transport's panic) outside Shutdown ends the process so the
// supervisor sees it instead of a panel without an API (90156b8d).
func (s *Server) Start() {
	defer func() {
		if r := recover(); r != nil && !s.stopping.Load() {
			logger.Errorf("server listener failed: %v", r)
			os.Exit(1)
		}
	}()
	if err := s.h.Run(); err != nil && !s.stopping.Load() {
		logger.Errorf("server listener failed: %s", err.Error())
		os.Exit(1)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.stopping.Store(true)
	return s.h.Shutdown(ctx)
}

func (s *Server) Engine() *server.Hertz {
	return s.h
}
