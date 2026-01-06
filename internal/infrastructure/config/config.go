package config

import (
	"fmt"
	"os"
	"strings"
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
	EPG    EPGConfig    `yaml:"epg"`
	UI     UIConfig     `yaml:"ui"`
}

// EPGConfig contains settings and saved searches related to EPG.
type EPGConfig struct {
	Searches []EPGSearch `yaml:"searches"`
}

// EPGSearch represents a saved EPG search definition.
// This is executed client-side against SVDRP EPG data (it does not require vdr-plugin-epgsearch).
type EPGSearch struct {
	ID          int    `yaml:"id"`
	Active      bool   `yaml:"active"`
	Pattern     string `yaml:"pattern"`
	Mode        string `yaml:"mode"` // "phrase" (default) or "regex"
	MatchCase   bool   `yaml:"match_case"`
	InTitle     bool   `yaml:"in_title"`
	InSubtitle  bool   `yaml:"in_subtitle"`
	InDesc      bool   `yaml:"in_description"`
	UseChannel  string `yaml:"use_channel"`  // "no" (default), "single", "range"
	ChannelID   string `yaml:"channel_id"`   // when UseChannel=="single"
	ChannelFrom string `yaml:"channel_from"` // when UseChannel=="range"
	ChannelTo   string `yaml:"channel_to"`   // when UseChannel=="range"
	// Reserved for future fields (time range, duration, day-of-week).
}

// NormalizeEPGSearch normalizes a single EPG search definition in-place.
// It mirrors the normalization performed in Config.Validate().
func NormalizeEPGSearch(s *EPGSearch) {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.Mode) == "" {
		s.Mode = "phrase"
	}
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	s.UseChannel = strings.ToLower(strings.TrimSpace(s.UseChannel))
	if s.UseChannel == "" {
		s.UseChannel = "no"
	}
	// Default search scope: title+subtitle+description.
	if !s.InTitle && !s.InSubtitle && !s.InDesc {
		s.InTitle, s.InSubtitle, s.InDesc = true, true, true
	}
}

// ValidateEPGSearch validates an EPG search definition.
// It is intentionally narrower than Config.Validate() so it can be safely used
// by endpoints like "Run" which accept untrusted user input.
func ValidateEPGSearch(s EPGSearch) error {
	if s.ID < 0 {
		return fmt.Errorf("invalid id: %d", s.ID)
	}
	if s.UseChannel != "no" && s.UseChannel != "single" && s.UseChannel != "range" {
		return fmt.Errorf("invalid use_channel: %q", s.UseChannel)
	}
	if s.Mode != "phrase" && s.Mode != "regex" {
		return fmt.Errorf("invalid mode: %q", s.Mode)
	}
	return nil
}

// UIConfig contains user interface settings
type UIConfig struct {
	// Theme controls the default theme: "system" (default), "light", or "dark".
	Theme string `yaml:"theme"`
	// LoginPage controls which page is shown after login / when clicking the top-left brand.
	// It must be a known path like "/timers".
	LoginPage string `yaml:"login_page"`
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
	DVBCards       int           `yaml:"dvb_cards"`
	WantedChannels []string      `yaml:"wanted_channels"`
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
	DefaultPriority    int `yaml:"default_priority"`
	DefaultLifetime    int `yaml:"default_lifetime"`
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
			DVBCards:       1,
			WantedChannels: []string{},
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
			RecordingExpiry: 0,
		},
		Timer: TimerConfig{
			DefaultPriority:    50,
			DefaultLifetime:    99,
			DefaultMarginStart: 2,
			DefaultMarginEnd:   10,
		},
		EPG: EPGConfig{
			Searches: []EPGSearch{},
		},
		UI: UIConfig{
			Theme:     "system",
			LoginPage: "/timers",
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

	if c.VDR.DVBCards < 1 || c.VDR.DVBCards > 99 {
		return fmt.Errorf("invalid vdr dvb_cards: %d (must be 1-99)", c.VDR.DVBCards)
	}

	// Normalize wanted channels: empty means "all channels".
	seen := map[string]struct{}{}
	clean := make([]string, 0, len(c.VDR.WantedChannels))
	for _, raw := range c.VDR.WantedChannels {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		clean = append(clean, v)
	}
	c.VDR.WantedChannels = clean

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

	// Normalize/validate UI login page.
	c.UI.LoginPage = strings.TrimSpace(c.UI.LoginPage)
	if c.UI.LoginPage == "" {
		c.UI.LoginPage = "/timers"
	}
	switch c.UI.LoginPage {
	case "/", "/now", "/channels", "/playing", "/timers", "/recordings", "/search", "/epgsearch", "/configurations":
		// ok
	default:
		return fmt.Errorf("invalid ui.login_page: %q", c.UI.LoginPage)
	}

	// Normalize/validate saved EPG searches.
	maxID := 0
	seenID := map[int]struct{}{}
	for i := range c.EPG.Searches {
		s := &c.EPG.Searches[i]
		if s.ID < 0 {
			return fmt.Errorf("invalid epg.searches[%d].id: %d", i, s.ID)
		}
		if s.ID > 0 {
			if _, ok := seenID[s.ID]; ok {
				return fmt.Errorf("duplicate epg search id: %d", s.ID)
			}
			seenID[s.ID] = struct{}{}
			if s.ID > maxID {
				maxID = s.ID
			}
		}

		NormalizeEPGSearch(s)
		if err := ValidateEPGSearch(*s); err != nil {
			// Keep error messages stable for existing configs.
			msg := err.Error()
			if strings.HasPrefix(msg, "invalid use_channel") {
				return fmt.Errorf("invalid epg.searches[%d].use_channel: %q", i, s.UseChannel)
			}
			if strings.HasPrefix(msg, "invalid mode") {
				return fmt.Errorf("invalid epg.searches[%d].mode: %q", i, s.Mode)
			}
			if strings.HasPrefix(msg, "invalid id") {
				return fmt.Errorf("invalid epg.searches[%d].id: %d", i, s.ID)
			}
			return fmt.Errorf("invalid epg.searches[%d]: %w", i, err)
		}
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
