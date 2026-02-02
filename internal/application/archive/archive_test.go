package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVDRInfo_Movie(t *testing.T) {
	info := `C X
T Familie Heinz Becker - Lachgeschichten
D Something
`
	got, err := ParseVDRInfo(strings.NewReader(info))
	if err != nil {
		t.Fatalf("ParseVDRInfo error: %v", err)
	}
	if got.Kind != KindMovie {
		t.Fatalf("kind=%q, want %q", got.Kind, KindMovie)
	}
	if got.Title != "Familie Heinz Becker - Lachgeschichten" {
		t.Fatalf("title=%q", got.Title)
	}
	if got.Episode != "" {
		t.Fatalf("episode=%q", got.Episode)
	}
}

func TestParseVDRInfo_Series(t *testing.T) {
	info := `T Tim und Struppi
S Im Reich des schwarzen Goldes (1)
`
	got, err := ParseVDRInfo(strings.NewReader(info))
	if err != nil {
		t.Fatalf("ParseVDRInfo error: %v", err)
	}
	if got.Kind != KindSeries {
		t.Fatalf("kind=%q, want %q", got.Kind, KindSeries)
	}
	if got.Title != "Tim und Struppi" {
		t.Fatalf("title=%q", got.Title)
	}
	if got.Episode != "Im Reich des schwarzen Goldes (1)" {
		t.Fatalf("episode=%q", got.Episode)
	}
}

func TestDiscoverSegments_SortsAndFiltersTS(t *testing.T) {
	dir := t.TempDir()
	files := []string{"00002.ts", "00001.ts", "note.txt", "00003.TS"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	segs, err := DiscoverSegments(dir)
	if err != nil {
		t.Fatalf("DiscoverSegments: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	if filepath.Base(segs[0]) != "00001.ts" || filepath.Base(segs[1]) != "00002.ts" || filepath.Base(segs[2]) != "00003.TS" {
		t.Fatalf("unexpected order: %v", []string{filepath.Base(segs[0]), filepath.Base(segs[1]), filepath.Base(segs[2])})
	}
}

func TestWriteConcatList_FormatsLines(t *testing.T) {
	dir := t.TempDir()
	seg1 := filepath.Join(dir, "a.ts")
	seg2 := filepath.Join(dir, "b.ts")
	if err := os.WriteFile(seg1, []byte("x"), 0644); err != nil {
		t.Fatalf("write seg: %v", err)
	}
	if err := os.WriteFile(seg2, []byte("x"), 0644); err != nil {
		t.Fatalf("write seg: %v", err)
	}
	listPath, err := WriteConcatList(dir, []string{seg1, seg2})
	if err != nil {
		t.Fatalf("WriteConcatList: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(listPath) })
	b, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read list: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "file '") {
		t.Fatalf("expected file lines, got: %q", got)
	}
	if !strings.Contains(got, seg1) || !strings.Contains(got, seg2) {
		t.Fatalf("expected segment paths in list, got: %q", got)
	}
}

func TestWriteConcatList_EscapesSpecialChars(t *testing.T) {
	dir := t.TempDir()
	// Simulate a path with apostrophe like "Moni's_Grill".
	subdir := filepath.Join(dir, "Moni's_Grill")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seg := filepath.Join(subdir, "00001.ts")
	if err := os.WriteFile(seg, []byte("x"), 0644); err != nil {
		t.Fatalf("write seg: %v", err)
	}

	listPath, err := WriteConcatList(dir, []string{seg})
	if err != nil {
		t.Fatalf("WriteConcatList: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(listPath) })

	b, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read list: %v", err)
	}
	got := string(b)
	// Apostrophe must be escaped as '\'' for ffmpeg concat demuxer.
	if !strings.Contains(got, "Moni'\\''s_Grill") {
		t.Fatalf("expected escaped apostrophe in concat list, got: %q", got)
	}
}

func TestParseProgressLine(t *testing.T) {
	k, v, ok := parseProgressLine("out_time_ms=123")
	if !ok || k != "out_time_ms" || v != "123" {
		t.Fatalf("unexpected parse: %v %q %q", ok, k, v)
	}
	_, _, ok = parseProgressLine("nope")
	if ok {
		t.Fatalf("expected false")
	}
}
func TestSlugify(t *testing.T) {
	in := "Familie Heinz Becker - Lachgeschichten"
	got := Slugify(in)
	want := "familie_heinz_becker_-_lachgeschichten"
	if got != want {
		t.Fatalf("Slugify=%q, want %q", got, want)
	}

	in2 := "ÄÖÜ äöü ß! (1)"
	got2 := Slugify(in2)
	want2 := "aeoeue_aeoeue_ss_1"
	if got2 != want2 {
		t.Fatalf("Slugify=%q, want %q", got2, want2)
	}
}

func TestJobManager_ActiveJobIDForRecording(t *testing.T) {
	m := NewJobManager()

	// Completed job should not be considered active.
	m.jobs["done"] = &Job{id: "done", recordingID: "rec-1", status: JobSuccess, created: time.Unix(10, 0)}

	// Active jobs for other recording.
	m.jobs["other"] = &Job{id: "other", recordingID: "rec-2", status: JobRunning, created: time.Unix(20, 0)}

	// Active jobs for target recording; latest should win.
	m.jobs["old"] = &Job{id: "old", recordingID: "rec-1", status: JobQueued, created: time.Unix(30, 0)}
	m.jobs["new"] = &Job{id: "new", recordingID: "rec-1", status: JobRunning, created: time.Unix(40, 0)}

	got, ok := m.ActiveJobIDForRecording("rec-1")
	if !ok {
		t.Fatalf("expected ok")
	}
	if got != "new" {
		t.Fatalf("jobID=%q, want %q", got, "new")
	}

	_, ok = m.ActiveJobIDForRecording("missing")
	if ok {
		t.Fatalf("expected not ok")
	}
}

func TestJobManager_ActiveJobIDsByRecording(t *testing.T) {
	m := NewJobManager()
	m.jobs["a1"] = &Job{id: "a1", recordingID: "rec-a", status: JobQueued, created: time.Unix(10, 0)}
	m.jobs["a2"] = &Job{id: "a2", recordingID: "rec-a", status: JobRunning, created: time.Unix(20, 0)}
	m.jobs["b"] = &Job{id: "b", recordingID: "rec-b", status: JobRunning, created: time.Unix(15, 0)}
	m.jobs["c"] = &Job{id: "c", recordingID: "rec-c", status: JobFailed, created: time.Unix(99, 0)}

	got := m.ActiveJobIDsByRecording()
	if got["rec-a"] != "a2" {
		t.Fatalf("rec-a jobID=%q, want %q", got["rec-a"], "a2")
	}
	if got["rec-b"] != "b" {
		t.Fatalf("rec-b jobID=%q, want %q", got["rec-b"], "b")
	}
	if _, ok := got["rec-c"]; ok {
		t.Fatalf("did not expect failed job to be included")
	}
}
