package svdrp_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
)

func TestClient_GetEPG_FallsBackWhenTimestampUnsupported(t *testing.T) {
	at := time.Unix(1700000000, 0)
	channelID := "C-1-2-3"

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "LSTE C-1-2-3 1700000000", respond: []string{"501 unknown option"}},
			{expect: "LSTE C-1-2-3", respond: []string{
				"250-C 1 C-1-2-3 SomeChannel:provider",
				"250-E 42 1700000000 3600",
				"250-T A Title",
				"250 end",
			}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := c.GetEPG(ctx, channelID, at)
	if err != nil {
		t.Fatalf("GetEPG: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Title != "A Title" {
		t.Fatalf("expected title %q, got %q", "A Title", events[0].Title)
	}
	if events[0].ChannelID != channelID {
		t.Fatalf("expected channel id %q, got %q", channelID, events[0].ChannelID)
	}
}

func TestClient_GetRecordings_FiltersMissingDirectories(t *testing.T) {
	base := t.TempDir()
	existsDir := filepath.Join(base, "rec1")
	if err := mkdirAll(existsDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	missingDir := filepath.Join(base, "does-not-exist")

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "UPDR", respond: []string{"250 updated"}},
			{expect: "LSTR", respond: []string{
				"250-1 02.01.26 20:15 1:30* CH~Title One~Sub~Desc",
				"250-2 02.01.26 19:00 0:45 CH~Title Two",
				"250 end",
			}},
			{expect: "LSTR 1 path", respond: []string{"250 " + existsDir}},
			{expect: "LSTR 2 path", respond: []string{"250 " + missingDir}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recs, err := c.GetRecordings(ctx)
	if err != nil {
		t.Fatalf("GetRecordings: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recs))
	}
	if recs[0].Path != "1" {
		t.Fatalf("expected recording id/path %q, got %q", "1", recs[0].Path)
	}
	if recs[0].Title != "Title One" {
		t.Fatalf("expected title %q, got %q", "Title One", recs[0].Title)
	}
}

func TestClient_GetRecordings_FiltersWhenPathLookupNotFound(t *testing.T) {
	base := t.TempDir()
	existsDir := filepath.Join(base, "rec1")
	if err := mkdirAll(existsDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "UPDR", respond: []string{"250 updated"}},
			{expect: "LSTR", respond: []string{
				"250-1 02.01.26 20:15 1:30* CH~Title One~Sub~Desc",
				"250-2 02.01.26 19:00 0:45 CH~Title Two",
				"250 end",
			}},
			{expect: "LSTR 1 path", respond: []string{"250 " + existsDir}},
			// Simulate stale VDR in-memory entry: path lookup fails.
			{expect: "LSTR 2 path", respond: []string{"550 recording not found"}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	recs, err := c.GetRecordings(ctx)
	if err != nil {
		t.Fatalf("GetRecordings: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recs))
	}
	if recs[0].Path != "1" {
		t.Fatalf("expected recording id/path %q, got %q", "1", recs[0].Path)
	}
}

func TestClient_GetChannels_RetriesOnTransientDisconnect(t *testing.T) {
	// First connection: accept LSTC then drop connection before responding.
	// Second connection: return a normal channels list.
	srv := newSVDRPTestServer(t, []svdrpConnScript{
		{steps: []svdrpConnStep{{expect: "LSTC", closeAfterRead: true}}},
		{steps: []svdrpConnStep{{expect: "LSTC", respond: []string{
			"250-1 C-1-2-3 SomeChannel:provider",
			"250 1 channels",
		}}}},
	})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	chs, err := c.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}
	if chs[0].ID != "C-1-2-3" {
		t.Fatalf("expected channel id %q, got %q", "C-1-2-3", chs[0].ID)
	}
	if chs[0].Name != "SomeChannel" {
		t.Fatalf("expected channel name %q, got %q", "SomeChannel", chs[0].Name)
	}
}

func mkdirAll(path string) error {
	// keep test helper small; avoid extra dependencies
	return mkdirAllMode(path, 0o755)
}
