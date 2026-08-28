package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpAdapter "github.com/githubixx/vdradmin-go/internal/adapters/primary/mcp"
	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	transport := flag.String("transport", "stdio", "MCP transport: stdio or http")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vdradmin-mcp v%s (%s %s)\n", version, commit, date)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	vdrClient := svdrp.NewClient(cfg.VDR.Host, cfg.VDR.Port, cfg.VDR.Timeout)
	defer func() {
		if err := vdrClient.Close(); err != nil {
			logger.Warn("failed to close VDR connection", slog.Any("error", err))
		}
	}()

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	if err := vdrClient.Connect(connectCtx); err != nil {
		logger.Warn("failed to connect to VDR; continuing with lazy connection", slog.Any("error", err))
	} else {
		logger.Info("connected to VDR")
	}
	cancelConnect()

	epgService := services.NewEPGService(vdrClient, cfg.Cache.EPGExpiry)
	epgService.SetWantedChannels(cfg.VDR.WantedChannels)
	server := mcpAdapter.NewServer(epgService, version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch *transport {
	case "stdio":
		logger.Info("starting MCP server", slog.String("transport", "stdio"), slog.String("version", version))
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("MCP server stopped with an error", slog.Any("error", err))
			os.Exit(1)
		}
	case "http":
		if err := runHTTP(ctx, cfg.MCP.Host, cfg.MCP.Port, server, logger); err != nil {
			logger.Error("MCP HTTP server stopped with an error", slog.Any("error", err))
			os.Exit(1)
		}
	default:
		logger.Error("unsupported MCP transport", slog.String("transport", *transport))
		os.Exit(2)
	}
}

func runHTTP(ctx context.Context, host string, port int, mcpServer *mcp.Server, logger *slog.Logger) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{Stateless: true, Logger: logger})
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting MCP server", slog.String("transport", "http"), slog.String("address", "http://"+httpServer.Addr+"/mcp"))
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
