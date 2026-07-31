package http

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestSetupRoutesServesStaticFilesFromConfiguredDirectory(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "app.css"), []byte("body { color: red; }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	router := SetupRoutes(&Handler{}, &config.AuthConfig{}, slog.Default(), staticDir)
	request := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Body.String(), "body { color: red; }\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
