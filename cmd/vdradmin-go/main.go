package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	httpAdapter "github.com/githubixx/vdradmin-go/internal/adapters/primary/http"
	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/theme"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	assetsDir := flag.String("assets-dir", "web", "path to the web asset directory")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vdradmin-go v%s (%s %s)\n", version, commit, date)
		return
	}

	templatesDir, staticDir, themesDir, err := assetDirectories(*assetsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid assets directory: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("starting vdradmin-go", slog.String("version", version), slog.String("commit", commit), slog.String("date", date))

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

	vdrClient := svdrp.NewClient(cfg.VDR.Host, cfg.VDR.Port, cfg.VDR.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := vdrClient.Connect(ctx); err != nil {
		logger.Warn("failed to connect to VDR (continuing without it)", slog.Any("error", err))
	} else {
		logger.Info("connected to VDR")
	}
	cancel()

	epgService := services.NewEPGService(vdrClient, cfg.Cache.EPGExpiry)
	epgService.SetWantedChannels(cfg.VDR.WantedChannels)
	timerService := services.NewTimerService(vdrClient)
	recordingService := services.NewRecordingService(vdrClient, cfg.Cache.RecordingExpiry)
	autoTimerService := services.NewAutoTimerService(vdrClient, timerService, epgService)

	themeManager := theme.NewManager(themesDir)
	if err := themeManager.Discover(); err != nil {
		logger.Error("failed to discover themes", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("themes discovered", slog.Any("themes", themeManager.GetAvailableThemes()))

	templates, err := loadTemplates(templatesDir)
	if err != nil {
		logger.Error("failed to load templates", slog.Any("error", err))
		os.Exit(1)
	}
	baseTemplate := templates["index.html"]

	httpHandler := httpAdapter.NewHandler(logger, baseTemplate, epgService, timerService, recordingService, autoTimerService)
	httpHandler.SetConfig(cfg, *configPath)
	httpHandler.SetVDRClient(vdrClient)
	httpHandler.SetTemplates(templates)
	httpHandler.SetUIThemeDefault(cfg.UI.Theme)
	httpHandler.SetThemeManager(themeManager)

	mux := httpAdapter.SetupRoutes(httpHandler, &cfg.Auth, logger, staticDir)
	server := httpAdapter.NewServer(&cfg.Server, logger, httpHandler, mux)

	go func() {
		if err := server.Start(); err != nil {
			logger.Error("server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	logger.Info("server started", slog.String("addr", fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down...")
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

func assetDirectories(assetsDir string) (templatesDir, staticDir, themesDir string, err error) {
	assetsDir = filepath.Clean(assetsDir)
	for _, name := range []string{"templates", "static", "themes"} {
		path := filepath.Join(assetsDir, name)
		info, statErr := os.Stat(path)
		if statErr != nil {
			return "", "", "", fmt.Errorf("%s: %w", path, statErr)
		}
		if !info.IsDir() {
			return "", "", "", fmt.Errorf("%s is not a directory", path)
		}
	}
	return filepath.Join(assetsDir, "templates"), filepath.Join(assetsDir, "static"), filepath.Join(assetsDir, "themes"), nil
}

func loadTemplates(templatesDir string) (map[string]*template.Template, error) {
	pages := []string{"index.html", "epg.html", "playing.html", "watch.html", "timers.html", "timer_edit.html", "recordings.html", "recording_archive.html", "recording_archive_jobs.html", "recording_archive_job.html", "recording_archive_job_status.html", "archive_profiles.html", "ffmpeg_profiles.html", "search.html", "search_results.html", "epgsearch.html", "epgsearch_edit.html", "epgsearch_results.html", "event.html", "channels.html", "configurations.html"}
	templates := make(map[string]*template.Template, len(pages))
	navigationTemplate := filepath.Join(templatesDir, "_nav.html")
	for _, page := range pages {
		tmpl, err := template.ParseFiles(navigationTemplate, filepath.Join(templatesDir, page))
		if err != nil {
			return nil, err
		}
		templates[page] = tmpl
	}
	return templates, nil
}
