package config

import (
	"fmt"
	"os"
	"time"

	"go.yaml.in/yaml/v4"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `yaml:"server"`
	VDR    VDRConfig    `yaml:"vdr"`
	Auth   AuthConfig   `yaml:"auth"`
	Cache  CacheConfig  `yaml:"cache"`
	Timer  TimerConfig  `yaml:"timer"`
	UI     UIConfig     `yaml:"ui"`
}

// UIConfig contains user interface settings
type UIConfig struct {
	// Theme controls the default theme: "system" (default), "light", or "dark".
	Theme string `yaml:"theme"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes"`
	TLS            TLSConfig     `yaml:"tls"`
}

// TLSConfig contains TLS settings
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// VDRConfig contains VDR connection settings
type VDRConfig struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	Timeout        time.Duration `yaml:"timeout"`
	VideoDir       string        `yaml:"video_dir"`
	ConfigDir      string        `yaml:"config_dir"`
	ReconnectDelay time.Duration `yaml:"reconnect_delay"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Enabled      bool     `yaml:"enabled"`
	AdminUser    string   `yaml:"admin_user"`
	AdminPass    string   `yaml:"admin_pass"`
	GuestEnabled bool     `yaml:"guest_enabled"`
	GuestUser    string   `yaml:"guest_user"`
	GuestPass    string   `yaml:"guest_pass"`
	LocalNets    []string `yaml:"local_nets"`
}

// CacheConfig contains caching settings
type CacheConfig struct {
	EPGExpiry       time.Duration `yaml:"epg_expiry"`
	RecordingExpiry time.Duration `yaml:"recording_expiry"`
}

// TimerConfig contains default timer settings
type TimerConfig struct {
	DefaultPriority   int `yaml:"default_priority"`
	DefaultLifetime   int `yaml:"default_lifetime"`
	DefaultMarginStart int `yaml:"default_margin_start"`
	DefaultMarginEnd   int `yaml:"default_margin_end"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	// Set defaults
	cfg := &Config{
		Server: ServerConfig{
			Host:           "0.0.0.0",
			Port:           8080,
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			MaxHeaderBytes: 1 << 20, // 1 MB
		},
		VDR: VDRConfig{
			Host:           "localhost",
			Port:           6419,
			Timeout:        10 * time.Second,
			VideoDir:       "/var/lib/video.00",
			ConfigDir:      "/etc/vdr",
			ReconnectDelay: 5 * time.Second,
		},
		Auth: AuthConfig{
			Enabled:      true,
			AdminUser:    "admin",
			AdminPass:    "admin",
			GuestEnabled: false,
			LocalNets:    []string{},
		},
		Cache: CacheConfig{
			EPGExpiry:       60 * time.Minute,
			RecordingExpiry: 5 * time.Minute,
		},
		Timer: TimerConfig{
			DefaultPriority:   50,
			DefaultLifetime:   99,
			DefaultMarginStart: 2,
			DefaultMarginEnd:   10,
		},
		UI: UIConfig{
			Theme: "system",
		},
	}

	// If config file exists, load it
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return cfg, nil // Use defaults if file doesn't exist
			}
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.VDR.Port < 1 || c.VDR.Port > 65535 {
		return fmt.Errorf("invalid VDR port: %d", c.VDR.Port)
	}

	if c.VDR.Host == "" {
		return fmt.Errorf("VDR host is required")
	}

	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertFile == "" {
			return fmt.Errorf("TLS cert file is required when TLS is enabled")
		}
		if c.Server.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}

	if c.Auth.Enabled {
		if c.Auth.AdminUser == "" {
			return fmt.Errorf("admin user is required when auth is enabled")
		}
		if c.Auth.AdminPass == "" {
			return fmt.Errorf("admin password is required when auth is enabled")
		}
	}

	if c.Timer.DefaultPriority < 0 || c.Timer.DefaultPriority > 99 {
		return fmt.Errorf("invalid default priority: %d (must be 0-99)", c.Timer.DefaultPriority)
	}

	if c.Timer.DefaultLifetime < 0 || c.Timer.DefaultLifetime > 99 {
		return fmt.Errorf("invalid default lifetime: %d (must be 0-99)", c.Timer.DefaultLifetime)
	}

	switch c.UI.Theme {
	case "", "system", "light", "dark":
		// ok
	default:
		return fmt.Errorf("invalid ui.theme: %q (must be system, light, or dark)", c.UI.Theme)
	}

	return nil
}

// Save saves the configuration to a YAML file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
