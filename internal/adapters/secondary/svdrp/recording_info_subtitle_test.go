package svdrp

import (
	"os"
	"testing"
)

func TestReadRecordingInfoSubtitle(t *testing.T) {
	dir := t.TempDir()
	// Note: the helper reads <dir>/info
	info := "T Title\nS Episode name\nD Desc\n"
	if err := writeFile(dir+"/info", info); err != nil {
		t.Fatalf("write info: %v", err)
	}
	sub, err := readRecordingInfoSubtitle(dir)
	if err != nil {
		t.Fatalf("readRecordingInfoSubtitle: %v", err)
	}
	if sub != "Episode name" {
		t.Fatalf("subtitle=%q, want %q", sub, "Episode name")
	}
}

func TestReadRecordingInfoSubtitle_EmptyWhenMissing(t *testing.T) {
	dir := t.TempDir()
	info := "T Title\nD Desc\n"
	if err := writeFile(dir+"/info", info); err != nil {
		t.Fatalf("write info: %v", err)
	}
	sub, err := readRecordingInfoSubtitle(dir)
	if err != nil {
		t.Fatalf("readRecordingInfoSubtitle: %v", err)
	}
	if sub != "" {
		t.Fatalf("subtitle=%q, want empty", sub)
	}
}

func writeFile(path string, content string) error {
	// small helper to keep tests compact
	return os.WriteFile(path, []byte(content), 0644)
}
