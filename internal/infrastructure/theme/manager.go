package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// Theme represents a UI theme with metadata.
type Theme struct {
	Name        string `yaml:"name"`
	Author      string `yaml:"author"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Path        string `yaml:"-"` // Directory path
}

// Manager handles theme discovery and loading.
type Manager struct {
	mu     sync.RWMutex
	themes map[string]*Theme
	dir    string // themes directory path
}

// NewManager creates a new theme manager.
func NewManager(themesDir string) *Manager {
	return &Manager{
		themes: make(map[string]*Theme),
		dir:    themesDir,
	}
}

// Discover scans the themes directory and loads all available themes.
func (m *Manager) Discover() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reset themes map
	m.themes = make(map[string]*Theme)

	// Check if themes directory exists
	if _, err := os.Stat(m.dir); os.IsNotExist(err) {
		return fmt.Errorf("themes directory does not exist: %s", m.dir)
	}

	// Read theme directories
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return fmt.Errorf("failed to read themes directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		themeName := entry.Name()
		themePath := filepath.Join(m.dir, themeName)

		// Load theme metadata
		theme, err := m.loadTheme(themePath)
		if err != nil {
			// Log warning but continue with other themes
			fmt.Printf("Warning: failed to load theme %q: %v\n", themeName, err)
			continue
		}

		m.themes[themeName] = theme
	}

	return nil
}

// loadTheme loads theme metadata from theme.yaml file.
func (m *Manager) loadTheme(path string) (*Theme, error) {
	metadataPath := filepath.Join(path, "theme.yaml")

	// Check if theme.yaml exists
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("theme.yaml not found")
	}

	// Read metadata file
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme.yaml: %w", err)
	}

	// Parse YAML
	var theme Theme
	if err := yaml.Unmarshal(data, &theme); err != nil {
		return nil, fmt.Errorf("failed to parse theme.yaml: %w", err)
	}

	theme.Path = path
	return &theme, nil
}

// GetTheme returns a theme by name.
func (m *Manager) GetTheme(name string) (*Theme, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	theme, ok := m.themes[name]
	return theme, ok
}

// GetAvailableThemes returns a list of all available theme names.
func (m *Manager) GetAvailableThemes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.themes))
	for name := range m.themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsValidTheme checks if a theme name is valid (exists or is "system").
func (m *Manager) IsValidTheme(name string) bool {
	if name == "system" {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.themes[name]
	return ok
}

// Reload re-discovers all themes (useful for hot-reload).
func (m *Manager) Reload() error {
	return m.Discover()
}
