// Command server runs the real web application against an isolated browser-test VDR.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	httpAdapter "github.com/githubixx/vdradmin-go/internal/adapters/primary/http"
	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/theme"
)

const fixturePassword = "test-password"

type fixtureServer struct {
	mu           sync.Mutex
	handler      http.Handler
	vdr          *fixtureVDR
	cfg          *config.Config
	ui           *httpAdapter.Handler
	epg          *services.EPGService
	recordings   *services.RecordingService
	themes       *theme.Manager
	storage      string
	configPath   string
	initialTheme string
	logger       *slog.Logger
}

func loadTemplates(dir string) (map[string]*template.Template, error) {
	// Keep the page set and shared partial identical to cmd/vdradmin-go/main.go.
	pages := []string{"index.html", "epg.html", "playing.html", "watch.html", "timers.html", "timer_edit.html", "recordings.html", "recording_archive.html", "recording_archive_jobs.html", "recording_archive_job.html", "recording_archive_job_status.html", "archive_profiles.html", "ffmpeg_profiles.html", "search.html", "search_results.html", "epgsearch.html", "epgsearch_edit.html", "epgsearch_results.html", "event.html", "channels.html", "configurations.html"}
	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		tmpl, err := httpAdapter.ParsePageTemplates(filepath.Join(dir, "_nav.html"), filepath.Join(dir, page))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		templates[page] = tmpl
	}
	return templates, nil
}

// Find the root from either the repository cwd or this package's test cwd.
func projectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("cannot locate project go.mod")
		}
		dir = parent
	}
}

func newFixture(root, initialTheme string, logger *slog.Logger) (_ *fixtureServer, err error) {
	themes := theme.NewManager(filepath.Join(root, "web", "themes"))
	defer func() {
		if err != nil {
			if closeErr := themes.Close(); closeErr != nil {
				logger.Error("close fixture theme overrides", "error", closeErr)
			}
		}
	}()
	if err := themes.Discover(); err != nil {
		return nil, fmt.Errorf("discover themes: %w", err)
	}
	if !themes.IsValidTheme(initialTheme) {
		return nil, fmt.Errorf("unknown theme %q", initialTheme)
	}
	templates, err := loadTemplates(filepath.Join(root, "web", "templates"))
	if err != nil {
		return nil, err
	}
	// Scratch storage is project-local; no host VDR/configuration path is used.
	storage := filepath.Join(root, "test", "e2e", "server", ".runtime-"+rand.Text())
	if err := os.Mkdir(storage, 0700); err != nil {
		return nil, fmt.Errorf("create fixture storage: %w", err)
	}
	defer func() {
		if err != nil {
			if cleanupErr := os.RemoveAll(storage); cleanupErr != nil {
				logger.Error("fixture setup cleanup failed", "error", cleanupErr)
			}
		}
	}()
	f := &fixtureServer{
		themes: themes, storage: storage, configPath: filepath.Join(storage, "config.yaml"),
		initialTheme: initialTheme, logger: logger,
	}
	f.cfg, err = f.defaultConfig()
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{f.cfg.VDR.VideoDir, f.cfg.VDR.ConfigDir, f.cfg.Archive.BaseDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	if err := f.cfg.Save(f.configPath); err != nil {
		return nil, fmt.Errorf("save fixture config: %w", err)
	}
	f.vdr = newFixtureVDR(f.cfg.VDR.VideoDir, time.Now())
	f.epg = services.NewEPGService(f.vdr, time.Hour)
	timerService := services.NewTimerService(f.vdr)
	f.recordings = services.NewRecordingService(f.vdr, time.Hour)
	autoTimers := services.NewAutoTimerService(f.vdr, timerService, f.epg)
	f.ui = httpAdapter.NewHandler(logger, templates["index.html"], f.epg, timerService, f.recordings, autoTimers)
	f.ui.SetTemplates(templates)
	f.ui.SetConfig(f.cfg, f.configPath)
	f.ui.SetVDRClient(f.vdr)
	f.ui.SetThemeManager(themes)
	f.ui.SetUIThemeDefault(initialTheme)
	f.handler = httpAdapter.SetupRoutes(f.ui, &f.cfg.Auth, logger, filepath.Join(root, "web", "static"))
	return f, nil
}

func (f *fixtureServer) defaultConfig() (*config.Config, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("load default config: %w", err)
	}
	cfg.Auth = config.AuthConfig{
		Enabled: true, AdminUser: "admin", AdminPass: fixturePassword,
		GuestEnabled: true, GuestUser: "viewer", GuestPass: fixturePassword,
	}
	cfg.UI.Theme = f.initialTheme
	cfg.VDR.Host = "fixture.invalid"
	cfg.VDR.DVBCards = 2
	cfg.VDR.VideoDir = filepath.Join(f.storage, "video")
	cfg.VDR.ConfigDir = filepath.Join(f.storage, "vdr-config")
	cfg.VDR.StreamURLTemplate = ""
	cfg.VDR.StreamdevBackendURL = ""
	cfg.Archive.BaseDir = filepath.Join(f.storage, "archive")
	cfg.Archive.Profiles = []config.ArchiveProfileConfig{
		{ID: "movies", Name: "Fixture movies", Kind: "movie", BaseDir: filepath.Join(cfg.Archive.BaseDir, "movies")},
		{ID: "series", Name: "Fixture series", Kind: "series", BaseDir: filepath.Join(cfg.Archive.BaseDir, "series")},
	}
	cfg.EPG.Searches = []config.EPGSearch{
		{ID: 1, Active: true, Pattern: "Planet", Mode: "phrase", InTitle: true, UseChannel: "no"},
		{ID: 2, Active: false, Pattern: "News", Mode: "phrase", InTitle: true, UseChannel: "no"},
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate fixture config: %w", err)
	}
	return cfg, nil
}

func (f *fixtureServer) Close() error {
	return errors.Join(f.themes.Close(), os.RemoveAll(f.storage))
}

func (f *fixtureServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serialize both test controls and normal requests: real handler config setters
	// and production config forms mutate the same pointer used by middleware.
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/__test/") {
		f.serveControl(w, r)
		return
	}
	if r.URL.Path == "/recordings/archive/start" || r.URL.Path == "/recordings/archive/preview" {
		http.Error(w, "fixture does not execute ffmpeg or archive media", http.StatusNotImplemented)
		return
	}
	// Production trusts loopback unconditionally. Use a documentation-only peer
	// address so browser tests exercise real Basic Auth and viewer permissions.
	r = r.Clone(r.Context())
	r.RemoteAddr = "192.0.2.1:12345"
	if r.URL.Path == "/watch/now" {
		r = r.WithContext(context.WithValue(r.Context(), currentProgrammeKey{}, true))
	}
	f.handler.ServeHTTP(w, r)
}

func (f *fixtureServer) serveControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/__test/ready":
		if r.Method != http.MethodGet {
			controlMethodError(w, http.MethodGet)
			return
		}
		f.writeJSON(w, f.metadata())
	case "/__test/theme":
		if r.Method != http.MethodPost {
			controlMethodError(w, http.MethodPost)
			return
		}
		value, err := controlField(w, r, "theme")
		if err != nil || !f.themes.IsValidTheme(value) {
			http.Error(w, "invalid theme", http.StatusBadRequest)
			return
		}
		old := f.cfg.UI.Theme
		f.cfg.UI.Theme = value
		if err := f.cfg.Save(f.configPath); err != nil {
			f.cfg.UI.Theme = old
			f.controlError(w, err)
			return
		}
		f.ui.SetUIThemeDefault(value)
		f.writeJSON(w, map[string]any{"ok": true, "theme": value})
	case "/__test/reset":
		if r.Method != http.MethodPost {
			controlMethodError(w, http.MethodPost)
			return
		}
		cfg, err := f.defaultConfig()
		if err != nil {
			f.controlError(w, err)
			return
		}
		if err := cfg.Save(f.configPath); err != nil {
			f.controlError(w, err)
			return
		}
		*f.cfg = *cfg
		f.ui.SetUIThemeDefault(f.initialTheme)
		f.epg.SetWantedChannels(nil)
		f.epg.SetCacheExpiry(time.Hour)
		f.recordings.SetCacheExpiry(time.Hour)
		f.vdr.reset()
		f.epg.InvalidateAllCaches()
		f.recordings.InvalidateCache()
		f.writeJSON(w, f.metadata())
	case "/__test/guide":
		if r.Method != http.MethodPost {
			controlMethodError(w, http.MethodPost)
			return
		}
		scenario, err := controlField(w, r, "scenario")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := f.vdr.setGuideScenario(scenario); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.epg.InvalidateAllCaches()
		f.writeJSON(w, map[string]any{"ok": true, "scenario": scenario})
	case "/__test/watch":
		if r.Method != http.MethodPost {
			controlMethodError(w, http.MethodPost)
			return
		}
		state, err := controlField(w, r, "state")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := f.vdr.setWatchState(state); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.writeJSON(w, map[string]any{"ok": true, "state": state})
	default:
		http.NotFound(w, r)
	}
}

func controlMethodError(w http.ResponseWriter, method string) {
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func controlField(w http.ResponseWriter, r *http.Request, field string) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		values := map[string]string{}
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&values); err != nil {
			return "", err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return "", errors.New("expected exactly one JSON object")
		}
		return strings.TrimSpace(values[field]), nil
	}
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.PostForm.Get(field)), nil
}

func (f *fixtureServer) controlError(w http.ResponseWriter, err error) {
	f.logger.Error("fixture control failed", "error", err)
	http.Error(w, "fixture control failed: "+err.Error(), http.StatusInternalServerError)
}

func (f *fixtureServer) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		f.logger.Error("fixture JSON response failed", "error", err)
	}
}

func (f *fixtureServer) metadata() map[string]any {
	timerDay := f.vdr.today.AddDate(0, 0, 1).Format("2006-01-02")
	return map[string]any{
		"ok": true, "date": fixtureDate, "day": fixtureDate, "fixtureDate": fixtureDate,
		"theme": f.cfg.UI.Theme, "themes": f.themes.GetAvailableThemes(),
		"timerDay": timerDay, "expiresAt": f.vdr.today.AddDate(0, 0, 1).Format(time.RFC3339),
		"eventURL":    "/event?channel=S19.2E-1-100-1&id=101",
		"channelName": "Das Erste HD", "eventTitle": "Evening News",
		"watchNowTitle": liveProgrammeTitle,
		"credentials": map[string]any{
			"admin":  map[string]string{"username": "admin", "password": fixturePassword},
			"viewer": map[string]string{"username": "viewer", "password": fixturePassword},
		},
		"urls": map[string]string{
			"playing":    "/playing?day=" + fixtureDate + "&start=18:00&end=23:59",
			"channels":   "/channels?channel=S19.2E-1-100-1&day=" + fixtureDate,
			"timers":     "/timers?day=" + timerDay,
			"recordings": "/recordings?sort=name", "watch": "/watch?interval=0&size=quarter",
			"watchPopup":      "/watch?interval=0&size=quarter&new_win=1",
			"watchFullscreen": "/watch?interval=0&size=quarter&full_tv=1",
			"search":          "/search?q=Planet",
		},
		"selectors": map[string]string{
			"timer": ".timer-item", "timerRecording": ".timer-item.recording",
			"timerInactive": ".timer-item.inactive", "timerOverlap": ".timer-item.collision:not(.conflict)",
			"timerConflict": ".timer-item.conflict", "watchImage": "#watchtv-image",
			"watchChannel": "#watchtv-channel", "watchOverlay": "#watchtv-overlay",
			"watchNowTitle": "#watchtv-now-title", "watchNowTime": "#watchtv-now-time",
		},
		"storagePathNormalization": map[string]any{
			"prefix": f.storage, "replacement": "/fixture",
			"inputSelectors": []string{"#vdr_video_dir", "#vdr_config_dir", "#archive_base_dir"},
			"note":           "Replace this prefix only in screenshot DOM values/text, including archive profile BaseDir table cells; never submit normalized paths.",
		},
		"normalize": []string{
			"Timer dates are relative to server startup day: mask .timer-time and select#day on /timers.",
			"Timer titles/order/clock labels and recording/collision/conflict classes are stable throughout the startup day; only calendar dates vary across startups.",
			"Timer editor date defaults, /now date/time, footer year, and configuration storage paths use the real server clock/environment.",
			"Fixed EPG entries all end at the next fixture midnight, keeping every Channels row visible throughout the fixture day.",
			"Watch /watch/now always exposes Fixture Live Programme for the startup day, midnight to midnight. There is no progress field. In matching browser/server timezones its visible time is always 00:00-00:00; otherwise normalize only #watchtv-now-time for timezone/DST differences.",
			"Restart the fixture after expiresAt (server-local midnight) to preserve recording state.",
		},
		"unavailable": []string{"real VDR/network streaming", "HLS/ffmpeg transcoding", "archive execution/preview", "recording media files"},
	}
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func run() error {
	addr := flag.String("addr", envDefault("E2E_ADDR", "127.0.0.1:18080"), "loopback HTTP listen address (E2E_ADDR)")
	themeID := flag.String("theme", envDefault("E2E_THEME", "light"), "initial theme ID (E2E_THEME)")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("invalid --addr: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("fixture must bind a loopback IP address")
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	root, err := projectRoot()
	if err != nil {
		return err
	}
	f, err := newFixture(root, *themeID, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			logger.Error("fixture storage cleanup failed", "error", err)
		}
	}()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: f, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	defer close(done)
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("fixture shutdown failed", "error", err)
			}
		case <-done:
		}
	}()
	logger.Info("browser fixture ready", "url", "http://"+listener.Addr().String(), "theme", *themeID, "fixtureDate", fixtureDate)
	serveErr := server.Serve(listener)
	if ctx.Err() != nil {
		<-shutdownDone
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("browser fixture failed", "error", err)
		os.Exit(1)
	}
}
