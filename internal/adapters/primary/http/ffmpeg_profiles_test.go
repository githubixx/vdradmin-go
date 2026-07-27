package http

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestConfigurationsFFMpegProfilesSave_PersistsProfilesAndDeletion(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	h := &Handler{
		cfg:        cfg,
		configPath: path,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{
		"ffmpeg_profile_indices":   {"0", "1"},
		"ffmpeg_profile_name_0":    {"Software"},
		"ffmpeg_profile_comment_0": {"Reliable CPU encoding"},
		"ffmpeg_profile_args_0":    {"-c:v libx264 -c:a aac"},
		"ffmpeg_profile_default_0": {"on"},
		"ffmpeg_profile_name_1":    {"VAAPI"},
		"ffmpeg_profile_args_1":    {"-c:v hevc_vaapi"},
		"ffmpeg_profile_delete_1":  {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configurations/ffmpeg-profiles/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.ConfigurationsFFMpegProfilesSave(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := cfg.Archive.FFMpegProfiles; len(got) != 1 || got[0].Name != "Software" || got[0].Comment != "Reliable CPU encoding" || !got[0].Default {
		t.Fatalf("runtime ffmpeg profiles = %#v, want only the saved default profile", got)
	}
	persisted, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load persisted config: %v", err)
	}
	if got := persisted.Archive.FFMpegProfiles; len(got) != 1 || got[0].Comment != "Reliable CPU encoding" || got[0].Args != "-c:v libx264 -c:a aac" {
		t.Fatalf("persisted ffmpeg profiles = %#v, want saved profile", got)
	}
}

func TestSelectedFFMpegProfile_UsesNamedProfileOrDefault(t *testing.T) {
	h := &Handler{}
	cfg := &config.Config{Archive: config.ArchiveConfig{FFMpegProfiles: []config.FFMpegProfileConfig{
		{Name: "Software", Comment: "Reliable CPU encoding", Args: "-c:v libx264"},
		{Name: "VAAPI", Comment: "Hardware acceleration", Args: "-c:v hevc_vaapi", Default: true},
	}}}

	profile, name, ok := h.selectedFFMpegProfile(cfg, "Software")
	if !ok || name != "Software" || profile.Comment != "Reliable CPU encoding" || profile.Args != "-c:v libx264" {
		t.Fatalf("named profile = %#v, %q, %t", profile, name, ok)
	}
	profile, name, ok = h.selectedFFMpegProfile(cfg, "unknown")
	if !ok || name != "VAAPI" || profile.Comment != "Hardware acceleration" || profile.Args != "-c:v hevc_vaapi" {
		t.Fatalf("fallback profile = %#v, %q, %t", profile, name, ok)
	}
}

func TestFFMpegProfileTemplatesParse(t *testing.T) {
	_, err := template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "configurations.html"),
		filepath.Join(repoRoot(t), "web", "templates", "ffmpeg_profiles.html"),
		filepath.Join(repoRoot(t), "web", "templates", "recording_archive.html"),
	)
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
}
