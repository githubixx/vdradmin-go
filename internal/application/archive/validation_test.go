package archive

import (
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		shouldSucceed bool
	}{
		{
			name:          "Normal path",
			path:          "/var/lib/video/Recording1/001.ts",
			shouldSucceed: true,
		},
		{
			name:          "Path with ..",
			path:          "/var/lib/video/../../../etc/passwd",
			shouldSucceed: false,
		},
		{
			name:          "Path with .. in middle",
			path:          "/var/lib/video/Recording1/../../../etc/passwd",
			shouldSucceed: false,
		},
		{
			name:          "Empty path",
			path:          "",
			shouldSucceed: true, // validatePath only checks for ..
		},
		{
			name:          "Relative path",
			path:          "Recording1/001.ts",
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for path with '..' but got success")
				}
			}
		})
	}
}

func TestValidatePreview(t *testing.T) {
	profile := ArchiveProfile{BaseDir: "/vdr/36/movies"}
	tests := []struct {
		name          string
		preview       Preview
		shouldSucceed bool
	}{
		{
			name: "configured root descendant",
			preview: Preview{
				TargetDir:   "/vdr/36/movies/example",
				VideoPath:   "/vdr/36/movies/example/video.mkv",
				InfoDstPath: "/vdr/36/movies/example/video.info",
			},
			shouldSucceed: true,
		},
		{
			name: "normalizes redundant separators",
			preview: Preview{
				TargetDir:   "/vdr/36/movies//example",
				VideoPath:   "/vdr/36/movies/example//video.mkv",
				InfoDstPath: "/vdr/36/movies/example//video.info",
			},
			shouldSucceed: true,
		},
		{
			name:    "relative target directory",
			preview: Preview{TargetDir: "example"},
		},
		{
			name:    "target outside configured root",
			preview: Preview{TargetDir: "/tmp/example"},
		},
		{
			name:    "sibling prefix escape",
			preview: Preview{TargetDir: "/vdr/36/movies-old/example"},
		},
		{
			name: "video outside target directory",
			preview: Preview{
				TargetDir:   "/vdr/36/movies/example",
				VideoPath:   "/vdr/36/movies/other/video.mkv",
				InfoDstPath: "/vdr/36/movies/example/video.info",
			},
		},
		{
			name: "info outside target directory",
			preview: Preview{
				TargetDir:   "/vdr/36/movies/example",
				VideoPath:   "/vdr/36/movies/example/video.mkv",
				InfoDstPath: "/vdr/36/movies/other/video.info",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidatePreview(profile, tt.preview, "mkv")
			if tt.shouldSucceed && err != nil {
				t.Fatalf("ValidatePreview() error: %v", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Fatal("ValidatePreview() succeeded, want error")
			}
		})
	}
}
