package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MigratesLegacyFFMpegArgsToDefaultProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("archive:\n  ffmpeg_args: -c:v libx264 -c:a copy\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Archive.FFMpegArgs; got != "" {
		t.Fatalf("legacy ffmpeg args = %q, want empty after migration", got)
	}
	if got := cfg.Archive.FFMpegProfiles; len(got) != 1 || got[0].Name != "Default" || got[0].Args != "-c:v libx264 -c:a copy" || !got[0].Default {
		t.Fatalf("ffmpeg profiles = %#v, want migrated default profile", got)
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "ffmpeg_args:") {
		t.Fatalf("saved config contains legacy ffmpeg_args:\n%s", data)
	}
}

func TestConfigValidate_FFMpegProfilesRequireExactlyOneDefault(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Archive.FFMpegProfiles = []FFMpegProfileConfig{
		{Name: "Software", Args: "-c:v libx264"},
		{Name: "VAAPI", Args: "-c:v hevc_vaapi"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly succeeded without a default ffmpeg profile")
	}

	cfg.Archive.FFMpegProfiles[0].Default = true
	cfg.Archive.FFMpegProfiles[1].Default = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly succeeded with two default ffmpeg profiles")
	}
}

func TestConfigValidate_FFMpegProfilesRejectDuplicateNames(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Archive.FFMpegProfiles = []FFMpegProfileConfig{
		{Name: "Software", Default: true},
		{Name: "software"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate unexpectedly succeeded with duplicate ffmpeg profile names")
	}
}
