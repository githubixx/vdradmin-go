package config

import "testing"

func TestConfigValidate_MCP(t *testing.T) {
	cfg := validConfigForMCPTest()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected MCP config to be valid: %v", err)
	}

	cfg.MCP.Host = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected empty MCP host to use loopback default: %v", err)
	}
	if cfg.MCP.Host != "127.0.0.1" {
		t.Fatalf("expected loopback MCP host default, got %q", cfg.MCP.Host)
	}

	cfg = validConfigForMCPTest()
	cfg.MCP.Port = 65536
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an invalid MCP port to fail validation")
	}
}

func validConfigForMCPTest() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080},
		MCP:    MCPConfig{Host: "127.0.0.1", Port: 8081},
		VDR:    VDRConfig{Host: "localhost", Port: 6419, DVBCards: 1},
		Auth:   AuthConfig{Enabled: false},
		UI:     UIConfig{Theme: "system", LoginPage: "/timers"},
	}
}
