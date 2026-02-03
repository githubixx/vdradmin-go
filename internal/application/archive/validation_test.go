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
