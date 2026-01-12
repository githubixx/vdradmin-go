package svdrp

import (
	"testing"
	"time"
)

func TestParseRecording_VDR27_LSTR_WithFlaggedLengthToken(t *testing.T) {
	// Example from real svdrpsend output (SVDRP prefix removed by readResponseLocked):
	// "4479 18.07.25 20:13 4:22*! 50 Jahre Musikvideos"
	line := "4479 18.07.25 20:13 4:22*! 50 Jahre Musikvideos"

	rec, err := parseRecording(line)
	if err != nil {
		t.Fatalf("parseRecording: %v", err)
	}
	if rec.Path != "4479" {
		t.Fatalf("Path=%q, want %q", rec.Path, "4479")
	}
	if rec.Title != "50 Jahre Musikvideos" {
		t.Fatalf("Title=%q, want %q", rec.Title, "50 Jahre Musikvideos")
	}

	wantDate := time.Date(2025, 7, 18, 20, 13, 0, 0, time.Local)
	if !rec.Date.Equal(wantDate) {
		t.Fatalf("Date=%v, want %v", rec.Date, wantDate)
	}

	wantLen := 4*time.Hour + 22*time.Minute
	if rec.Length != wantLen {
		t.Fatalf("Length=%v, want %v", rec.Length, wantLen)
	}
}

func TestInferRecordingTitleFromDir_UsesShowFolder(t *testing.T) {
	dir := "/hdd01/vdr/video/Tuff_Stuff/_/2025-07-05.23.00.77-0.rec"
	got := inferRecordingTitleFromDir(dir)
	if got != "Tuff Stuff" {
		t.Fatalf("inferRecordingTitleFromDir=%q, want %q", got, "Tuff Stuff")
	}
}
