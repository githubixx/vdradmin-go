package http

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/theme"
)

func TestSharedAssetsAndLegacyArtwork(t *testing.T) {
	manager := theme.NewManager(filepath.Join(t.TempDir(), "missing"))
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}
	static := t.TempDir()
	handler := &Handler{logger: slog.Default(), themeManager: manager}
	router := SetupRoutes(handler, &config.AuthConfig{}, slog.Default(), static)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}
	for _, path := range []string{
		"/ui/css/base.css", "/ui/js/theme.js", "/ui/js/menu.js", "/ui/js/select.js",
		"/static/js/theme.js", "/static/js/custom-select.js", "/themes/light/theme.css",
		"/themes/crt-starfield-1/assets/crt-sky.webp", "/static/img/themes/crt-starfield-1/crt-sky.webp",
	} {
		got := get(path)
		if got.Code != http.StatusOK || got.Body.Len() == 0 {
			t.Fatalf("%s: %d %s", path, got.Code, got.Body.String())
		}
	}
	modern := get("/themes/crt-starfield-1/assets/crt-sky.webp")
	legacy := get("/static/img/themes/crt-starfield-1/crt-sky.webp")
	if modern.Body.String() != legacy.Body.String() || legacy.Header().Get("Content-Type") != "image/webp" {
		t.Fatal("legacy alias differs from embedded artwork")
	}
	file := filepath.Join(static, "img/themes/crt-starfield-1/crt-sky.webp")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("application replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := get("/static/img/themes/crt-starfield-1/crt-sky.webp"); got.Body.String() != "application replacement" {
		t.Fatal("application-local legacy artwork lost precedence")
	}
	for _, path := range []string{"/themes/light/theme.yaml", "/ui/themes/light/theme.yaml", "/themes/unknown/theme.css"} {
		if got := get(path); got.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, got.Code)
		}
	}
	protected := SetupRoutes(handler, &config.AuthConfig{Enabled: true, AdminUser: "admin", AdminPass: "password"}, slog.Default(), static)
	for _, path := range []string{"/themes/light/theme.css", "/themes/crt-starfield-1/assets/crt-sky.webp", "/ui/themes/light/theme.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.10:1234"
		got := httptest.NewRecorder()
		protected.ServeHTTP(got, req)
		if got.Code != http.StatusUnauthorized {
			t.Fatalf("unprotected theme route %s: %d", path, got.Code)
		}
	}
	if css := get("/themes/crt-starfield-1/theme.css").Body.String(); !strings.Contains(css, "assets/crt-sky.webp") {
		t.Fatal("legacy stylesheet lost relative artwork URL")
	}
}
