package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpAdapter "github.com/githubixx/vdradmin-go/internal/adapters/primary/http"
	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vdradmin-go v%s (%s %s)\n", version, commit, date)
		os.Exit(0)
	}

	// Setup logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("starting vdradmin-go", slog.String("version", version), slog.String("commit", commit), slog.String("date", date))

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		slog.String("vdr_host", cfg.VDR.Host),
		slog.Int("vdr_port", cfg.VDR.Port),
		slog.String("server_host", cfg.Server.Host),
		slog.Int("server_port", cfg.Server.Port),
	)

	// Initialize SVDRP client
	vdrClient := svdrp.NewClient(
		cfg.VDR.Host,
		cfg.VDR.Port,
		cfg.VDR.Timeout,
	)

	// Attempt an early connect to VDR (non-fatal).
	// The SVDRP client will also connect lazily on demand.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := vdrClient.Connect(ctx); err != nil {
		logger.Warn("failed to connect to VDR (continuing without it)", slog.Any("error", err))
	} else {
		logger.Info("connected to VDR")
	}
	cancel()

	// Initialize services
	epgService := services.NewEPGService(vdrClient, cfg.Cache.EPGExpiry)
	epgService.SetWantedChannels(cfg.VDR.WantedChannels)
	timerService := services.NewTimerService(vdrClient)
	recordingService := services.NewRecordingService(vdrClient, cfg.Cache.RecordingExpiry)
	autoTimerService := services.NewAutoTimerService(vdrClient, timerService, epgService)

	// Load templates - each page gets its own template set
	templates := make(map[string]*template.Template)
	pages := []string{"index.html", "epg.html", "playing.html", "watch.html", "timers.html", "timer_edit.html", "recordings.html", "recording_archive.html", "recording_archive_jobs.html", "recording_archive_job.html", "recording_archive_job_status.html", "archive_profiles.html", "search.html", "search_results.html", "epgsearch.html", "epgsearch_edit.html", "epgsearch_results.html", "event.html", "channels.html", "configurations.html"}

	for _, page := range pages {
		tmpl := template.Must(template.ParseFiles("web/templates/_nav.html", "web/templates/"+page))
		templates[page] = tmpl
	}

	// Convert map to single template (we'll handle lookup in handler)
	// For now, just use the first one as base
	baseTemplate := templates["index.html"]

	// Initialize HTTP handler (pass template map)
	httpHandler := httpAdapter.NewHandler(
		logger,
		baseTemplate, // We'll change Handler to accept map later
		epgService,
		timerService,
		recordingService,
		autoTimerService,
	)
	httpHandler.SetConfig(cfg, *configPath)
	httpHandler.SetVDRClient(vdrClient)

	// Set template map in handler
	httpHandler.SetTemplates(templates)
	httpHandler.SetUIThemeDefault(cfg.UI.Theme)

	// Setup routes
	mux := httpAdapter.SetupRoutes(httpHandler, &cfg.Auth, logger)

	// Create HTTP server
	server := httpAdapter.NewServer(&cfg.Server, logger, httpHandler, mux)

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			logger.Error("server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	logger.Info("server started",
		slog.String("addr", fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)),
	)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.Any("error", err))
	}

	if err := vdrClient.Close(); err != nil {
		logger.Error("failed to close VDR connection", slog.Any("error", err))
	}

	logger.Info("shutdown complete")
}
