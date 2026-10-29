package httpserver

import (
	"context"
	"crypto/tls"

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
	h *server.Hertz
}

type Dependencies struct {
	Routes           routes.Dependencies
	Notification     notification.Service
	TelegramBotToken func() string
	RequestMetadata  requestmeta.Enricher
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

	routes.RegisterHandlers(engine, deps.Routes)
	notificationHTTP.RegisterTelegramHandlers(engine, deps.Notification, deps.TelegramBotToken)
	billingHTTP.RegisterNotifyHandlers(engine, deps.Routes.Store, deps.Routes.Billing)

	return &Server{h: engine}
}

func (s *Server) Start() {
	if err := s.h.Run(); err != nil {
		logger.Errorf("server start error: %s", err.Error())
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.h.Shutdown(ctx)
}

func (s *Server) Engine() *server.Hertz {
	return s.h
}
