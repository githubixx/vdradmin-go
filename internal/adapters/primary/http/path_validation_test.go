package http

import (
	"path/filepath"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestIsPathWithinBase(t *testing.T) {
	tests := []struct {
		name          string
		baseDir       string
		requestedPath string
		shouldSucceed bool
		reason        string
	}{
		{
			name:          "Simple valid path",
			baseDir:       "/var/lib/video",
			requestedPath: "Recording1/2024-01-01.rec",
			shouldSucceed: true,
			reason:        "Normal recording path should be allowed",
		},
		{
			name:          "Path traversal with ../",
			baseDir:       "/var/lib/video",
			requestedPath: "../../etc/passwd",
			shouldSucceed: false,
			reason:        "Path traversal should be blocked",
		},
		{
			name:          "Path traversal in middle",
			baseDir:       "/var/lib/video",
			requestedPath: "Recording1/../../../etc/passwd",
			shouldSucceed: false,
			reason:        "Path traversal in middle should be blocked",
		},
		{
			name:          "Absolute path escaping base",
			baseDir:       "/var/lib/video",
			requestedPath: "/etc/passwd",
			shouldSucceed: false,
			reason:        "Absolute path outside base should be blocked",
		},
		{
			name:          "Empty base directory",
			baseDir:       "",
			requestedPath: "Recording1",
			shouldSucceed: false,
			reason:        "Empty base directory should fail",
		},
		{
			name:          "Empty requested path",
			baseDir:       "/var/lib/video",
			requestedPath: "",
			shouldSucceed: false,
			reason:        "Empty requested path should fail",
		},
		{
			name:          "Relative base directory",
			baseDir:       "video",
			requestedPath: "Recording1",
			shouldSucceed: false,
			reason:        "Relative base directory should fail",
		},
		{
			name:          "Deep nested valid path",
			baseDir:       "/var/lib/video",
			requestedPath: "a/b/c/d/e/recording.rec",
			shouldSucceed: true,
			reason:        "Deep nesting should be allowed if within base",
		},
		{
			name:          "Path exactly at base",
			baseDir:       "/var/lib/video",
			requestedPath: ".",
			shouldSucceed: false,
			reason:        "Path exactly at base (not in subdirectory) should fail",
		},
		{
			name:          "Multiple slashes",
			baseDir:       "/var/lib/video",
			requestedPath: "Recording1//recording.rec",
			shouldSucceed: true,
			reason:        "Multiple slashes are cleaned and should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isPathWithinBase(tt.baseDir, tt.requestedPath)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v (reason: %s)", err, tt.reason)
				}
				if result == "" {
					t.Errorf("Expected non-empty result path (reason: %s)", tt.reason)
				}
				// Verify the result is actually within the base
				if tt.baseDir != "" && filepath.IsAbs(tt.baseDir) {
					cleanBase := filepath.Clean(tt.baseDir)
					if result != cleanBase && !filepath.HasPrefix(result, cleanBase+string(filepath.Separator)) {
						t.Errorf("Result path %q is not within base %q", result, cleanBase)
					}
				}
			} else {
				if err == nil {
					t.Errorf("Expected error but got success with result: %s (reason: %s)", result, tt.reason)
				}
			}
		})
	}
}

func TestValidateRecordingPath(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{
			VDR: config.VDRConfig{
				VideoDir: "/var/lib/video.00",
			},
		},
	}

	tests := []struct {
		name          string
		recordingPath string
		shouldSucceed bool
		reason        string
	}{
		{
			name:          "Valid recording path",
			recordingPath: "Recording1/2024-01-01.rec",
			shouldSucceed: true,
			reason:        "Normal recording paths should be allowed",
		},
		{
			name:          "Path with ..",
			recordingPath: "../../../etc/passwd",
			shouldSucceed: false,
			reason:        "Paths with .. should be rejected",
		},
		{
			name:          "Path with backslash on Unix",
			recordingPath: "Recording1\\..\\..\\passwd",
			shouldSucceed: false,
			reason:        "Backslashes should be rejected on Unix systems",
		},
		{
			name:          "Deep nested recording",
			recordingPath: "Shows/Drama/Series/Season1/Episode1.rec",
			shouldSucceed: true,
			reason:        "Deep nesting should work",
		},
		{
			name:          "Empty path",
			recordingPath: "",
			shouldSucceed: false,
			reason:        "Empty paths should be rejected",
		},
		{
			name:          "Absolute path",
			recordingPath: "/etc/passwd",
			shouldSucceed: false,
			reason:        "Absolute paths that escape video dir should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validateRecordingPath(tt.recordingPath)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v (reason: %s)", err, tt.reason)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error but got success (reason: %s)", tt.reason)
				}
			}
		})
	}
}

func TestValidateRecordingPath_NoConfig(t *testing.T) {
	h := &Handler{
		cfg: nil,
	}

	err := h.validateRecordingPath("Recording1")
	if err == nil {
		t.Error("Expected error when config is nil")
	}
}

func TestValidateRecordingPath_NoVideoDir(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{
			VDR: config.VDRConfig{
				VideoDir: "",
			},
		},
	}

	err := h.validateRecordingPath("Recording1")
	if err == nil {
		t.Error("Expected error when video directory is not configured")
	}
}

func TestValidateRecordingDir(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{
			VDR: config.VDRConfig{
				VideoDir: "/var/lib/video.00",
			},
		},
	}

	tests := []struct {
		name          string
		recordingDir  string
		shouldSucceed bool
		reason        string
	}{
		{
			name:          "Valid recording directory",
			recordingDir:  "/var/lib/video.00/Recording1/2024-01-01.rec",
			shouldSucceed: true,
			reason:        "Absolute path within video dir should be allowed",
		},
		{
			name:          "Recording at video dir root",
			recordingDir:  "/var/lib/video.00",
			shouldSucceed: true,
			reason:        "Video dir itself should be allowed (recordings can be at root)",
		},
		{
			name:          "Path escaping video dir",
			recordingDir:  "/var/lib/other/recording",
			shouldSucceed: false,
			reason:        "Path outside video dir should be rejected",
		},
		{
			name:          "Path escaping with traversal",
			recordingDir:  "/var/lib/video.00/../../../etc/passwd",
			shouldSucceed: false,
			reason:        "Cleaned path that escapes should be rejected",
		},
		{
			name:          "Empty path",
			recordingDir:  "",
			shouldSucceed: false,
			reason:        "Empty path should be rejected",
		},
		{
			name:          "Deep nested recording",
			recordingDir:  "/var/lib/video.00/Shows/Drama/Series/Season1/Episode1.rec",
			shouldSucceed: true,
			reason:        "Deep nesting within video dir should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validateRecordingDir(tt.recordingDir)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v (reason: %s)", err, tt.reason)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error but got success (reason: %s)", tt.reason)
				}
			}
		})
	}
}
