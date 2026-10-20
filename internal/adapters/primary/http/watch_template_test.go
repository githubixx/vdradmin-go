package http

import (
	"bytes"
	"html/template"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTemplate_Watch_LiveDurationInfinity(t *testing.T) {
	tmpl := template.Must(ParsePageTemplates(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "watch.html"),
	))
	hlsConfig := regexp.MustCompile(`new window\.Hls\(\{[^}]*\bliveDurationInfinity:\s*true\s*,`)

	for _, tc := range []struct {
		name      string
		streamURL string
	}{
		{name: "built-in HLS", streamURL: "/watch/stream/{channel}/index.m3u8"},
		{name: "external HLS", streamURL: "https://example.com/{channel}/index.m3u8"},
		{name: "snapshot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tmpl.ExecuteTemplate(&buf, "watch.html", map[string]any{
				"ThemeMode":         "light",
				"ThemeDefault":      "light",
				"StreamURLTemplate": tc.streamURL,
				"Interval":          "0",
				"Size":              "full",
			})
			if err != nil {
				t.Fatalf("execute template: %v", err)
			}

			body := buf.String()
			if !strings.Contains(body, `<script src="https://cdn.jsdelivr.net/npm/hls.js@1.7.3/dist/hls.min.js"></script>`) {
				t.Fatal("expected the pinned hls.js 1.7.3 script")
			}
			if !hlsConfig.MatchString(body) {
				t.Fatal("expected hls.js to report indefinite live stream duration")
			}
			hasVideo := strings.Contains(body, `<video id="watchtv-video"`)
			hasSnapshot := strings.Contains(body, `<img id="watchtv-image"`)
			streaming := tc.streamURL != ""
			if hasVideo != streaming || hasSnapshot == streaming {
				t.Fatal("expected video only in streaming mode and image only in snapshot mode")
			}
		})
	}
}
