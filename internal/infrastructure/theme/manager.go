package theme

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	chromaui "github.com/githubixx/chroma-ui"
)

// Theme is Chroma's copy-safe display metadata.
type Theme = chromaui.Theme

// Manager adapts Chroma's catalog to the application's theme selection API.
// Once opened, the override directory is pinned until Close; edits within that
// directory remain live, but replacing the directory requires a new Manager.
type Manager struct {
	mu      sync.RWMutex
	catalog *chromaui.Catalog
	root    *os.Root
	dir     string
	closed  bool
}

// NewManager creates a manager; call Discover before serving theme assets.
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// Discover merges bundled themes with optional whole-theme directory overrides.
// Missing directories are optional; every other discovery error is fatal.
// A failed discovery preserves the previous metadata and source selection.
func (m *Manager) Discover() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return os.ErrClosed
	}
	if m.root != nil {
		return m.catalog.Reload()
	}

	root, err := os.OpenRoot(m.dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("open theme overrides %q: %w", m.dir, err)
	}

	var catalog *chromaui.Catalog
	if root == nil {
		catalog, err = chromaui.NewCatalog(nil)
	} else {
		catalog, err = chromaui.NewCatalog(root.FS())
	}
	if err != nil {
		if root != nil {
			err = errors.Join(err, root.Close())
		}
		return err
	}
	m.catalog, m.root = catalog, root
	return nil
}

// Reload revalidates all sources. An initially missing directory is retried.
func (m *Manager) Reload() error { return m.Discover() }

// GetAvailableThemes returns independent theme IDs sorted lexicographically.
func (m *Manager) GetAvailableThemes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0)
	if m.catalog != nil {
		for _, theme := range m.catalog.Themes() {
			ids = append(ids, theme.ID)
		}
	}
	return ids
}

// GetTheme returns a copy of the metadata for an exact theme ID.
func (m *Manager) GetTheme(id string) (*Theme, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.catalog == nil {
		return nil, false
	}
	theme, ok := m.catalog.Theme(id)
	if !ok {
		return nil, false
	}
	return &theme, true
}

// IsValidTheme accepts discovered themes and the application's system mode.
func (m *Manager) IsValidTheme(id string) bool {
	if id == "system" {
		return true
	}
	_, ok := m.GetTheme(id)
	return ok
}

// Handler forwards Chroma's /css, /js, and /themes path space unchanged.
// Mount with http.StripPrefix when using an application-specific URL prefix.
func (m *Manager) Handler(logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		if m.catalog == nil {
			logger.Error("theme catalog unavailable", "path", r.URL.Path)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		m.catalog.Handler(logger).ServeHTTP(w, r)
	})
}

// Close waits for in-flight requests and releases the override directory.
// It is idempotent and terminal: subsequent discovery returns os.ErrClosed,
// metadata lookups are empty, and asset requests fail explicitly.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	m.catalog = nil
	if m.root == nil {
		return nil
	}
	err := m.root.Close()
	m.root = nil
	return err
}
