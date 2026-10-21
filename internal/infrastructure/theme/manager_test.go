package theme

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func testDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".theme-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	return dir
}

func writeFile(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTheme(t *testing.T, dir, id, name, css string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, id, "theme.yaml"), "name: "+name+"\nauthor: Test\nversion: 1.0.0\n")
	writeFile(t, filepath.Join(dir, id, "theme.css"), css)
}

func discoveredManager(t *testing.T, dir string) *Manager {
	t.Helper()
	m := NewManager(dir)
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := m.Discover(); err != nil {
		t.Fatal(err)
	}
	return m
}

func request(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestEmbeddedAndLocalMerge(t *testing.T) {
	dir := testDirectory(t)
	m := discoveredManager(t, dir)
	bundled := m.GetAvailableThemes()
	if len(bundled) != 37 || !sort.StringsAreSorted(bundled) {
		t.Fatalf("expected 37 sorted embedded themes, got %v", bundled)
	}
	writeTheme(t, dir, "light", "Local Light", ":root { color: red; }")
	writeTheme(t, dir, "custom", "Custom", ":root { color: blue; }")
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	ids := m.GetAvailableThemes()
	if len(ids) != 38 || !sort.StringsAreSorted(ids) {
		t.Fatalf("expected merged sorted IDs, got %v", ids)
	}
	expectedIDs := append([]string(nil), ids...)
	theme, ok := m.GetTheme("light")
	if !ok || theme.Name != "Local Light" || theme.ID != "light" || theme.Author != "Test" {
		t.Fatalf("wrong override metadata: %+v, %v", theme, ok)
	}
	theme.Name = "Mutated"
	ids[0] = "mutated"
	fresh, _ := m.GetTheme("light")
	if fresh.Name != "Local Light" || !reflect.DeepEqual(expectedIDs, m.GetAvailableThemes()) {
		t.Fatal("returned metadata or IDs were not copied")
	}
	for id, want := range map[string]bool{"system": true, "light": true, "custom": true, "unknown": false, "": false, "LIGHT": false} {
		if m.IsValidTheme(id) != want {
			t.Errorf("IsValidTheme(%q) != %v", id, want)
		}
	}
	if theme, ok := m.GetTheme("unknown"); ok || theme != nil {
		t.Fatalf("missing theme = %+v, %v", theme, ok)
	}
}

func TestMissingDirectoryCanAppearOnReload(t *testing.T) {
	dir := filepath.Join(testDirectory(t), "later")
	m := discoveredManager(t, dir)
	handler := m.Handler(nil)
	if len(m.GetAvailableThemes()) != 37 {
		t.Fatal("missing optional directory did not use embedded themes")
	}
	if got := request(handler, http.MethodGet, "/themes/light/theme.css"); got.Code != http.StatusOK {
		t.Fatalf("embedded CSS: %d", got.Code)
	}
	writeTheme(t, dir, "light", "Appeared", ":root { color: green; }")
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := request(handler, http.MethodGet, "/themes/light/theme.css"); got.Body.String() != ":root { color: green; }" {
		t.Fatalf("existing handler missed new override: %d %q", got.Code, got.Body.String())
	}
}

func TestStrictInvalidOverrides(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*testing.T, string)
	}{
		{"malformed", func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "light/theme.yaml"), "name: [[[") }},
		{"unknown-field", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "light/theme.yaml"), "name: Light\nunknown: value\n")
		}},
		{"missing-name", func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "light/theme.yaml"), "author: Test\n") }},
		{"missing-metadata", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "light/theme.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing-css", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "light/theme.css")); err != nil {
				t.Fatal(err)
			}
		}},
		{"empty-css", func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "light/theme.css"), " \n") }},
		{"reserved-id", func(t *testing.T, dir string) { writeTheme(t, dir, "system", "System", ":root {}") }},
		{"invalid-id", func(t *testing.T, dir string) { writeTheme(t, dir, "Upper", "Upper", ":root {}") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testDirectory(t)
			writeTheme(t, dir, "light", "Local", ":root {}")
			tc.edit(t, dir)
			m := NewManager(dir)
			defer m.Close()
			if err := m.Discover(); err == nil {
				t.Fatal("invalid override silently accepted")
			}
			if len(m.GetAvailableThemes()) != 0 {
				t.Fatal("failed initial discovery published fallback metadata")
			}
		})
	}
	t.Run("not-directory", func(t *testing.T) {
		path := filepath.Join(testDirectory(t), "file")
		writeFile(t, path, "not a directory")
		m := NewManager(path)
		defer m.Close()
		if err := m.Discover(); err == nil {
			t.Fatal("non-directory accepted")
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can read directories without permission bits")
		}
		dir := testDirectory(t)
		if err := os.Chmod(dir, 0000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0755)
		m := NewManager(dir)
		defer m.Close()
		if err := m.Discover(); err == nil {
			t.Fatal("unreadable directory accepted")
		}
	})
}

func TestLiveAssetsAndAtomicReload(t *testing.T) {
	dir := testDirectory(t)
	writeTheme(t, dir, "light", "Original", ":root { color: red; }")
	m := discoveredManager(t, dir)
	handler := m.Handler(nil)
	before := m.GetAvailableThemes()
	writeFile(t, filepath.Join(dir, "light/theme.css"), ":root { color: blue; }")
	if got := request(handler, http.MethodGet, "/themes/light/theme.css"); got.Code != http.StatusOK || got.Body.String() != ":root { color: blue; }" {
		t.Fatalf("CSS edit not live: %d %q", got.Code, got.Body.String())
	}
	writeTheme(t, dir, "added", "Added", ":root {}")
	writeFile(t, filepath.Join(dir, "light/theme.yaml"), "name: Updated\n")
	writeTheme(t, dir, "broken", "Broken", "")
	if err := m.Reload(); err == nil {
		t.Fatal("invalid reload accepted")
	}
	metadata, _ := m.GetTheme("light")
	if metadata.Name != "Original" || !reflect.DeepEqual(before, m.GetAvailableThemes()) {
		t.Fatal("failed reload changed metadata")
	}
	if got := request(handler, http.MethodGet, "/themes/added/theme.css"); got.Code != http.StatusNotFound {
		t.Fatal("failed reload published a new source")
	}
	if got := request(handler, http.MethodGet, "/themes/light/theme.css"); got.Body.String() != ":root { color: blue; }" {
		t.Fatal("failed reload lost local source selection")
	}
	if err := os.RemoveAll(filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	metadata, _ = m.GetTheme("light")
	if metadata.Name != "Updated" || !m.IsValidTheme("added") {
		t.Fatal("valid reload was not published")
	}
	if err := os.Remove(filepath.Join(dir, "light/theme.css")); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err == nil {
		t.Fatal("missing override CSS accepted")
	}
	if got := request(handler, http.MethodGet, "/themes/light/theme.css"); got.Code != http.StatusNotFound {
		t.Fatal("missing override CSS fell back to embedded CSS")
	}
}

func TestHandlerAssetsAndProtection(t *testing.T) {
	dir := testDirectory(t)
	writeTheme(t, dir, "light", "Local", ":root {}")
	writeFile(t, filepath.Join(dir, "light/assets/art.svg"), "<svg/>")
	writeFile(t, filepath.Join(dir, "light/assets/.private.png"), "private")
	writeFile(t, filepath.Join(dir, "light/assets/secret.yaml"), "private")
	m := discoveredManager(t, dir)
	mux := http.NewServeMux()
	mux.Handle("/ui/", http.StripPrefix("/ui", m.Handler(nil)))
	for _, path := range []string{"/css/base.css", "/js/theme.js", "/js/menu.js", "/js/select.js", "/themes/dark/theme.css", "/themes/light/theme.css", "/themes/light/assets/art.svg"} {
		got := request(mux, http.MethodGet, "/ui"+path)
		if got.Code != http.StatusOK || got.Body.Len() == 0 {
			t.Errorf("%s: %d %q", path, got.Code, got.Body.String())
		}
		if got.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing content protection", path)
		}
	}
	if got := request(mux, http.MethodGet, "/ui/themes/light/assets/art.svg"); got.Body.String() != "<svg/>" || got.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatal("artwork not forwarded")
	}
	// Whole-theme overrides never fall through to embedded artwork.
	if got := request(mux, http.MethodGet, "/ui/themes/light/assets/missing.svg"); got.Code != http.StatusNotFound {
		t.Fatal("missing local artwork did not return 404")
	}
	for _, path := range []string{"/themes/light/theme.yaml", "/themes/light/", "/themes/light/assets/", "/themes/light/assets/.private.png", "/themes/light/assets/secret.yaml", "/themes/light/assets/..%2ftheme.yaml", "/themes/light/assets/escape.svg"} {
		if got := request(m.Handler(nil), http.MethodGet, path); got.Code != http.StatusNotFound {
			t.Errorf("non-public %s returned %d", path, got.Code)
		}
	}
	head := request(mux, http.MethodHead, "/ui/themes/light/theme.css")
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatal("HEAD not forwarded")
	}
	if got := request(mux, http.MethodPost, "/ui/themes/light/theme.css"); got.Code != http.StatusMethodNotAllowed {
		t.Fatal("write method allowed")
	}
}

func TestSymlinkNamespaceSafety(t *testing.T) {
	dir := testDirectory(t)
	outside := testDirectory(t)
	writeTheme(t, dir, "custom", "Custom", ":root {}")
	writeFile(t, filepath.Join(outside, "secret.svg"), "<svg>secret</svg>")
	target, err := filepath.Abs(filepath.Join(outside, "secret.svg"))
	if err != nil {
		t.Fatal(err)
	}
	m := discoveredManager(t, dir)
	if err := os.Mkdir(filepath.Join(dir, "custom/assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "custom/assets/escape.svg")); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	got := request(m.Handler(slog.New(slog.NewTextHandler(&logs, nil))), http.MethodGet, "/themes/custom/assets/escape.svg")
	if got.Code != http.StatusInternalServerError || strings.Contains(got.Body.String(), "secret") || logs.Len() == 0 {
		t.Fatalf("symlink asset: %d %q, logs %q", got.Code, got.Body.String(), logs.String())
	}
	if err := os.Remove(filepath.Join(dir, "custom/theme.css")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "custom/theme.css")); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err == nil {
		t.Fatal("symlink stylesheet accepted")
	}
}

func TestUnavailableCatalogAndClose(t *testing.T) {
	m := NewManager(filepath.Join(testDirectory(t), "missing"))
	var logs bytes.Buffer
	handler := m.Handler(slog.New(slog.NewTextHandler(&logs, nil)))
	for _, path := range []string{"/css/base.css", "/themes/light/theme.css"} {
		if got := request(handler, http.MethodGet, path); got.Code != http.StatusInternalServerError {
			t.Fatalf("uninitialized catalog returned %d", got.Code)
		}
	}
	if !strings.Contains(logs.String(), "theme catalog unavailable") {
		t.Fatal("unavailable catalog not logged")
	}
	if err := m.Discover(); err != nil {
		t.Fatal(err)
	}
	if got := request(handler, http.MethodGet, "/css/base.css"); got.Code != http.StatusOK {
		t.Fatal("handler did not see discovery")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal("Close is not idempotent")
	}
	if !errors.Is(m.Discover(), os.ErrClosed) || !errors.Is(m.Reload(), os.ErrClosed) {
		t.Fatal("closed manager reopened")
	}
	if len(m.GetAvailableThemes()) != 0 || m.IsValidTheme("light") || !m.IsValidTheme("system") {
		t.Fatal("incorrect closed metadata semantics")
	}
	if got := request(handler, http.MethodGet, "/css/base.css"); got.Code != http.StatusInternalServerError {
		t.Fatal("closed handler returned success")
	}
}

type blockingWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.ResponseRecorder.Write(data)
}

func TestCloseWaitsForInFlightHandler(t *testing.T) {
	dir := testDirectory(t)
	writeTheme(t, dir, "custom", "Custom", ":root {}")
	m := discoveredManager(t, dir)
	root := m.root
	w := &blockingWriter{ResponseRecorder: httptest.NewRecorder(), started: make(chan struct{}), release: make(chan struct{})}
	served := make(chan struct{})
	go func() {
		defer close(served)
		m.Handler(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/themes/custom/theme.css", nil))
	}()
	<-w.started
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	select {
	case err := <-closed:
		close(w.release)
		t.Fatalf("Close interrupted an active handler: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(w.release)
	<-served
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK {
		t.Fatal("in-flight handler failed")
	}
	if _, err := root.Stat("."); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("root not closed: %v", err)
	}
}

func TestConcurrentAccessAndReload(t *testing.T) {
	dir := testDirectory(t)
	writeTheme(t, dir, "custom", "Custom", ":root {}")
	m := discoveredManager(t, dir)
	handler := m.Handler(nil)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				_ = m.GetAvailableThemes()
				value, ok := m.GetTheme("custom")
				if !ok || value.Name != "Custom" {
					t.Error("concurrent lookup failed")
					return
				}
				value.Name = "Private mutation"
				if !m.IsValidTheme("custom") {
					t.Error("concurrent validation failed")
				}
				if got := request(handler, http.MethodGet, "/themes/custom/theme.css"); got.Code != http.StatusOK {
					t.Errorf("concurrent handler returned %d", got.Code)
				}
			}
		})
	}
	wg.Go(func() {
		for i := 0; i < 10; i++ {
			if err := m.Reload(); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
}
