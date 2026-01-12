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
	mux.Handle("GET /now", chain(handler.WhatsOnNow, commonMiddleware...))
	mux.Handle("GET /channels", chain(handler.Channels, commonMiddleware...))
	mux.Handle("GET /configurations", chain(handler.Configurations, commonMiddleware...))

	// Config management sub-pages (admin-only)
	mux.Handle("GET /configurations/archive-profiles", chain(handler.ConfigurationsArchiveProfiles, adminMiddleware...))
	mux.Handle("POST /configurations/archive-profiles/save", chain(handler.ConfigurationsArchiveProfilesSave, adminMiddleware...))
	mux.Handle("GET /playing", chain(handler.PlayingToday, commonMiddleware...))
	mux.Handle("GET /watch", chain(handler.WatchTV, commonMiddleware...))
	mux.Handle("POST /watch/key", chain(handler.WatchTVKey, commonMiddleware...))
	mux.Handle("POST /watch/channel", chain(handler.WatchTVChannel, commonMiddleware...))
	mux.Handle("GET /watch/now", chain(handler.WatchTVNow, commonMiddleware...))
	mux.Handle("GET /watch/snapshot", chain(handler.WatchTVSnapshot, commonMiddleware...))
	mux.Handle("GET /watch/stream/{channel}/index.m3u8", chain(handler.WatchTVStreamPlaylist, commonMiddleware...))
	mux.Handle("GET /watch/stream/{channel}/{segment}", chain(handler.WatchTVStreamSegment, commonMiddleware...))
	mux.Handle("GET /epg", chain(handler.EPGList, commonMiddleware...))
	mux.Handle("GET /event", chain(handler.EventInfo, commonMiddleware...))
	mux.Handle("GET /search", chain(handler.EPGSearch, commonMiddleware...))
	mux.Handle("GET /epgsearch", chain(handler.EPGSearchList, commonMiddleware...))
	mux.Handle("POST /epgsearch/execute", chain(handler.EPGSearchExecute, commonMiddleware...))
	mux.Handle("GET /timers", chain(handler.TimerList, commonMiddleware...))
	mux.Handle("GET /recordings", chain(handler.RecordingList, commonMiddleware...))
	mux.Handle("POST /recordings/refresh", chain(handler.RecordingRefresh, commonMiddleware...))

	// Archive (admin-only for now)
	mux.Handle("GET /recordings/archive", chain(handler.RecordingArchivePrepare, adminMiddleware...))
	mux.Handle("GET /recordings/archive/preview", chain(handler.RecordingArchivePreview, adminMiddleware...))
	mux.Handle("POST /recordings/archive/start", chain(handler.RecordingArchiveStart, adminMiddleware...))
	mux.Handle("GET /recordings/archive/jobs", chain(handler.RecordingArchiveJobs, adminMiddleware...))
	mux.Handle("GET /recordings/archive/job", chain(handler.RecordingArchiveJob, adminMiddleware...))
	mux.Handle("GET /recordings/archive/job/poll", chain(handler.RecordingArchiveJobPoll, adminMiddleware...))
	mux.Handle("GET /recordings/archive/job/status", chain(handler.RecordingArchiveJobStatus, adminMiddleware...))
	mux.Handle("POST /recordings/archive/job/cancel", chain(handler.RecordingArchiveJobCancel, adminMiddleware...))

	// Admin-only routes (write operations)
	mux.Handle("POST /configurations/apply", chain(handler.ConfigurationsApply, adminMiddleware...))
	mux.Handle("POST /configurations/save", chain(handler.ConfigurationsSave, adminMiddleware...))
	mux.Handle("GET /epgsearch/new", chain(handler.EPGSearchNew, adminMiddleware...))
	mux.Handle("POST /epgsearch/new", chain(handler.EPGSearchCreate, adminMiddleware...))
	mux.Handle("GET /epgsearch/edit", chain(handler.EPGSearchEdit, adminMiddleware...))
	mux.Handle("POST /epgsearch/edit", chain(handler.EPGSearchUpdate, adminMiddleware...))
	mux.Handle("POST /epgsearch/delete", chain(handler.EPGSearchDelete, adminMiddleware...))
	mux.Handle("GET /timers/new", chain(handler.TimerNew, adminMiddleware...))
	mux.Handle("POST /timers/new", chain(handler.TimerCreateManual, adminMiddleware...))
	mux.Handle("GET /timers/edit", chain(handler.TimerEdit, adminMiddleware...))
	mux.Handle("POST /timers/create", chain(handler.TimerCreate, adminMiddleware...))
	mux.Handle("POST /timers/update", chain(handler.TimerUpdate, adminMiddleware...))
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
