package http

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"
)

func legacyThemeImagePath(path string) string {
	return "/themes/" + strings.Replace(strings.TrimPrefix(path, "/static/img/themes/"), "/", "/assets/", 1)
}

func (h *Handler) serveUI(w http.ResponseWriter, r *http.Request) {
	if h.themeManager == nil {
		h.logger.Error("theme manager not initialized")
		http.Error(w, "Theme manager not available", http.StatusInternalServerError)
		return
	}
	h.themeManager.Handler(h.logger).ServeHTTP(w, r)
}

func (h *Handler) sharedAsset(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request := r.Clone(r.Context())
		request.URL.Path = path
		h.serveUI(w, request)
	}
}

func (h *Handler) legacyThemeImage(staticDir string, static http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/static/")
		file, err := http.Dir(staticDir).Open(name)
		if err == nil {
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				h.logger.Error("inspect local theme artwork", "path", name, "error", err)
				http.Error(w, "Cannot read artwork", http.StatusInternalServerError)
				return
			}
			if info.IsDir() {
				http.NotFound(w, r)
				return
			}
			static.ServeHTTP(w, r)
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			h.logger.Error("read local theme artwork", "path", name, "error", err)
			http.Error(w, "Cannot read artwork", http.StatusInternalServerError)
			return
		}
		h.sharedAsset(legacyThemeImagePath(r.URL.Path))(w, r)
	}
}
