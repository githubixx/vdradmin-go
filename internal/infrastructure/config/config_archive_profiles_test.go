package config

import "testing"

func TestConfigValidate_ArchiveProfileIDNoneReserved(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Archive.Profiles = []ArchiveProfileConfig{{
		ID:      "none",
		Name:    "None",
		Kind:    "movie",
		BaseDir: "/tmp/archive",
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for reserved id 'none'")
	}
}
