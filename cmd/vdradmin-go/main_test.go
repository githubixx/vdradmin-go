package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetDirectories(t *testing.T) {
	assetsDir := t.TempDir()
	for _, name := range []string{"templates", "static", "themes"} {
		if err := os.Mkdir(filepath.Join(assetsDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	templatesDir, staticDir, themesDir, err := assetDirectories(assetsDir)
	if err != nil {
		t.Fatalf("assetDirectories() error = %v", err)
	}
	if templatesDir != filepath.Join(assetsDir, "templates") {
		t.Fatalf("templatesDir = %q", templatesDir)
	}
	if staticDir != filepath.Join(assetsDir, "static") {
		t.Fatalf("staticDir = %q", staticDir)
	}
	if themesDir != filepath.Join(assetsDir, "themes") {
		t.Fatalf("themesDir = %q", themesDir)
	}
}

func TestAssetDirectoriesRequiresAllAssetSubdirectories(t *testing.T) {
	assetsDir := t.TempDir()
	if _, _, _, err := assetDirectories(assetsDir); err == nil {
		t.Fatal("assetDirectories() error = nil, want missing directory error")
	}
}
