package http

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/theme"
)

func TestServeTheme(t *testing.T) {
	// Create temp directory with test themes
	tmpDir := t.TempDir()

	// Create test theme
	testDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "theme.yaml"), []byte(`
name: Test
author: test
description: Test theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}
	themeCSSContent := `/* Test Theme */
:root[data-theme="test"] {
    --primary-color: #ff0000;
}`
	if err := os.WriteFile(filepath.Join(testDir, "theme.css"), []byte(themeCSSContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup handler with theme manager
	handler := &Handler{
		logger: slog.Default(),
	}
	manager := theme.NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}
	handler.SetThemeManager(manager)

	tests := []struct {
		name         string
		themeName    string
		wantStatus   int
		wantContains string
	}{
		{
			name:         "existing theme",
			themeName:    "test",
			wantStatus:   http.StatusOK,
			wantContains: "--primary-color",
		},
		{
			name:       "non-existent theme",
			themeName:  "nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty theme name",
			themeName:  "",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/themes/"+tt.themeName+"/theme.css", nil)
			req.SetPathValue("name", tt.themeName)
			rr := httptest.NewRecorder()

			handler.ServeTheme(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("ServeTheme() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantContains != "" && rr.Code == http.StatusOK {
				body := rr.Body.String()
				if len(body) == 0 {
					t.Error("Expected non-empty response body")
				}
			}
		})
	}
}

func TestServeTheme_NoThemeManager(t *testing.T) {
	handler := &Handler{
		logger: slog.Default(),
	}
	// Don't set theme manager

	req := httptest.NewRequest("GET", "/themes/test/theme.css", nil)
	req.SetPathValue("name", "test")
	rr := httptest.NewRecorder()

	handler.ServeTheme(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 when theme manager not initialized, got %d", rr.Code)
	}
}

func TestServeTheme_MissingThemeCSS(t *testing.T) {
	tmpDir := t.TempDir()

	// Create theme with metadata but no CSS file
	testDir := filepath.Join(tmpDir, "nocss")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "theme.yaml"), []byte(`
name: NoCSS
author: test
description: Theme without CSS
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{
		logger: slog.Default(),
	}
	manager := theme.NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}
	handler.SetThemeManager(manager)

	req := httptest.NewRequest("GET", "/themes/nocss/theme.css", nil)
	req.SetPathValue("name", "nocss")
	rr := httptest.NewRecorder()

	handler.ServeTheme(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 when theme.css missing, got %d", rr.Code)
	}
}
