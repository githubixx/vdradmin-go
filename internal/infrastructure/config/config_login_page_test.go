package config

import "testing"

func TestConfigValidate_LoginPage(t *testing.T) {
	cfg := &Config{}
	// Minimal valid config: rely on defaults/zero-values that Validate accepts? We must
	// fill required fields to pass validation.
	cfg.Server.Port = 8080
	cfg.VDR.Host = "localhost"
	cfg.VDR.Port = 6419
	cfg.VDR.DVBCards = 1
	cfg.Auth.Enabled = false
	cfg.UI.Theme = "system"

	cfg.UI.LoginPage = "/timers"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected login_page to be valid: %v", err)
	}

	cfg.UI.LoginPage = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected empty login_page to be accepted/normalized: %v", err)
	}
	if cfg.UI.LoginPage != "/timers" {
		t.Fatalf("expected login_page to normalize to '/timers', got %q", cfg.UI.LoginPage)
	}

	cfg.UI.LoginPage = "/now"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected /now login_page to be valid: %v", err)
	}

	cfg.UI.LoginPage = "/nope"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid login_page to fail validation")
	}
}
