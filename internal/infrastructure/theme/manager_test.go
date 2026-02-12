package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Discover(t *testing.T) {
	// Create temp directory with test themes
	tmpDir := t.TempDir()

	// Create light theme
	lightDir := filepath.Join(tmpDir, "light")
	if err := os.MkdirAll(lightDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lightDir, "theme.yaml"), []byte(`
name: Light
author: test
description: Test light theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create dark theme
	darkDir := filepath.Join(tmpDir, "dark")
	if err := os.MkdirAll(darkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(darkDir, "theme.yaml"), []byte(`
name: Dark
author: test
description: Test dark theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create theme without metadata (should be skipped with warning)
	noMetaDir := filepath.Join(tmpDir, "nometa")
	if err := os.MkdirAll(noMetaDir, 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatalf("Expected Discover to succeed, got error: %v", err)
	}

	available := manager.GetAvailableThemes()
	if len(available) != 2 {
		t.Errorf("Expected 2 themes, got %d: %v", len(available), available)
	}

	// Check light theme
	lightTheme, ok := manager.GetTheme("light")
	if !ok {
		t.Error("Expected to find light theme")
	}
	if lightTheme.Name != "Light" {
		t.Errorf("Expected theme name 'Light', got %q", lightTheme.Name)
	}
	if lightTheme.Author != "test" {
		t.Errorf("Expected author 'test', got %q", lightTheme.Author)
	}

	// Check dark theme
	darkTheme, ok := manager.GetTheme("dark")
	if !ok {
		t.Error("Expected to find dark theme")
	}
	if darkTheme.Name != "Dark" {
		t.Errorf("Expected theme name 'Dark', got %q", darkTheme.Name)
	}

	// Check non-existent theme
	_, ok = manager.GetTheme("nonexistent")
	if ok {
		t.Error("Expected not to find nonexistent theme")
	}
}

func TestManager_IsValidTheme(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one test theme
	testDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "theme.yaml"), []byte(`
name: Test
author: test
description: Test theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		want      bool
		themeName string
	}{
		{"system is always valid", true, "system"},
		{"existing theme is valid", true, "test"},
		{"non-existent theme is invalid", false, "nonexistent"},
		{"empty string is invalid", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.IsValidTheme(tt.themeName)
			if got != tt.want {
				t.Errorf("IsValidTheme(%q) = %v, want %v", tt.themeName, got, tt.want)
			}
		})
	}
}

func TestManager_DiscoverNonExistentDirectory(t *testing.T) {
	manager := NewManager("/nonexistent/path")
	err := manager.Discover()
	if err == nil {
		t.Error("Expected error when discovering non-existent directory")
	}
}

func TestManager_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// Create theme with malformed YAML
	badDir := filepath.Join(tmpDir, "bad")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "theme.yaml"), []byte(`
name: Bad
this is not valid yaml: [[[
`), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(tmpDir)
	// Should not fail, just skip the bad theme
	if err := manager.Discover(); err != nil {
		t.Fatalf("Expected Discover to succeed despite bad theme, got error: %v", err)
	}

	available := manager.GetAvailableThemes()
	if len(available) != 0 {
		t.Errorf("Expected 0 themes (bad theme should be skipped), got %d", len(available))
	}
}

func TestManager_Reload(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial theme
	initialDir := filepath.Join(tmpDir, "initial")
	if err := os.MkdirAll(initialDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initialDir, "theme.yaml"), []byte(`
name: Initial
author: test
description: Initial theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}

	if len(manager.GetAvailableThemes()) != 1 {
		t.Errorf("Expected 1 theme initially, got %d", len(manager.GetAvailableThemes()))
	}

	// Add new theme
	newDir := filepath.Join(tmpDir, "new")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "theme.yaml"), []byte(`
name: New
author: test
description: New theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Reload
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}

	if len(manager.GetAvailableThemes()) != 2 {
		t.Errorf("Expected 2 themes after reload, got %d", len(manager.GetAvailableThemes()))
	}

	_, ok := manager.GetTheme("new")
	if !ok {
		t.Error("Expected to find new theme after reload")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test theme
	testDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "theme.yaml"), []byte(`
name: Test
author: test
description: Test theme
version: 1.0.0
`), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(tmpDir)
	if err := manager.Discover(); err != nil {
		t.Fatal(err)
	}

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				_ = manager.GetAvailableThemes()
				_, _ = manager.GetTheme("test")
				_ = manager.IsValidTheme("test")
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
