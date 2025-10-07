package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

// Server represents the HTTP server
type Server struct {
	config  *config.ServerConfig
	logger  *slog.Logger
	server  *http.Server
	handler *Handler
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.ServerConfig, logger *slog.Logger, handler *Handler, mux http.Handler) *Server {
	return &Server{
		config:  cfg,
		logger:  logger,
		handler: handler,
		server: &http.Server{
			Addr:           fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:        mux,
			ReadTimeout:    cfg.ReadTimeout,
			WriteTimeout:   cfg.WriteTimeout,
			MaxHeaderBytes: cfg.MaxHeaderBytes,
		},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server",
		slog.String("addr", s.server.Addr),
		slog.Bool("tls", s.config.TLS.Enabled),
	)

	if s.config.TLS.Enabled {
		return s.server.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.server.Shutdown(ctx)
}

// SetupRoutes configures all HTTP routes using Go 1.22+ routing
func SetupRoutes(handler *Handler, authCfg *config.AuthConfig, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Apply middleware chain
	chain := func(h http.HandlerFunc, middlewares ...func(http.Handler) http.Handler) http.Handler {
		handler := http.Handler(h)
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}

	// Common middleware for all routes
	commonMiddleware := []func(http.Handler) http.Handler{
		RecoveryMiddleware(logger),
		LoggingMiddleware(logger),
		SecurityHeadersMiddleware(),
		CompressionMiddleware(),
		AuthMiddleware(authCfg),
	}

	// Admin-only middleware
	adminMiddleware := append(commonMiddleware, RequireAdminMiddleware())

	// Public routes
	mux.Handle("GET /", chain(handler.Home, commonMiddleware...))
	mux.Handle("GET /epg", chain(handler.EPGList, commonMiddleware...))
	mux.Handle("GET /search", chain(handler.EPGSearch, commonMiddleware...))
	mux.Handle("GET /timers", chain(handler.TimerList, commonMiddleware...))
	mux.Handle("GET /recordings", chain(handler.RecordingList, commonMiddleware...))

	// Admin-only routes (write operations)
	mux.Handle("POST /timers/create", chain(handler.TimerCreate, adminMiddleware...))
	mux.Handle("POST /timers/toggle", chain(handler.TimerToggle, adminMiddleware...))
	mux.Handle("DELETE /timers", chain(handler.TimerDelete, adminMiddleware...))
	mux.Handle("POST /timers/delete", chain(handler.TimerDelete, adminMiddleware...)) // For browsers without DELETE
	mux.Handle("DELETE /recordings", chain(handler.RecordingDelete, adminMiddleware...))
	mux.Handle("POST /recordings/delete", chain(handler.RecordingDelete, adminMiddleware...)) // For browsers without DELETE

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))

	return mux
}
