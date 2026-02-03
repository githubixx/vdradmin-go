package services

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

func TestRecordingService_GetAllRecordings_NoCacheWhenExpiryZero(t *testing.T) {
	var calls int32
	client := &ports.MockVDRClient{
		GetRecordingsFunc: func(ctx context.Context) ([]domain.Recording, error) {
			atomic.AddInt32(&calls, 1)
			return []domain.Recording{{Path: "1", Title: "A"}}, nil
		},
	}

	svc := NewRecordingService(client, 0)
	ctx := context.Background()

	_, err := svc.GetAllRecordings(ctx)
	if err != nil {
		t.Fatalf("GetAllRecordings(1): %v", err)
	}
	_, err = svc.GetAllRecordings(ctx)
	if err != nil {
		t.Fatalf("GetAllRecordings(2): %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 backend calls, got %d", got)
	}
}

func TestRecordingService_SortRecordings_DefaultNewestFirst(t *testing.T) {
	svc := NewRecordingService(ports.NewMockVDRClient(), 0)

	recs := []domain.Recording{
		{Path: "b", Title: "B", Date: time.Date(2026, 1, 2, 10, 0, 0, 0, time.Local)},
		{Path: "a", Title: "A", Date: time.Date(2026, 1, 2, 11, 0, 0, 0, time.Local)},
		{Path: "c", Title: "C", Date: time.Date(2026, 1, 1, 11, 0, 0, 0, time.Local)},
	}

	sorted := svc.SortRecordings(recs, "")
	if len(sorted) != 3 {
		t.Fatalf("expected 3, got %d", len(sorted))
	}
	if sorted[0].Path != "a" {
		t.Fatalf("expected newest first (path a), got %q", sorted[0].Path)
	}
	if sorted[1].Path != "b" {
		t.Fatalf("expected second newest (path b), got %q", sorted[1].Path)
	}
	if sorted[2].Path != "c" {
		t.Fatalf("expected oldest last (path c), got %q", sorted[2].Path)
	}
}

func TestRecordingService_GetAllRecordings_PrunesMissingDiskPathOnCacheHit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vdradmin-rec-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	// Remove the temp dir at the end in case the test fails early.
	defer os.RemoveAll(tmpDir)

	// Create a minimal `info` file to represent a valid VDR recording directory.
	if err := os.WriteFile(filepath.Join(tmpDir, "info"), []byte("T Title\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(info): %v", err)
	}

	var calls int32
	client := &ports.MockVDRClient{
		GetRecordingsFunc: func(ctx context.Context) ([]domain.Recording, error) {
			atomic.AddInt32(&calls, 1)
			return []domain.Recording{{Path: "1", Title: "A", DiskPath: tmpDir}}, nil
		},
	}

	svc := NewRecordingService(client, time.Hour)
	ctx := context.Background()

	// First call should fetch and populate cache.
	recs, err := svc.GetAllRecordings(ctx)
	if err != nil {
		t.Fatalf("GetAllRecordings(1): %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recs))
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 backend call, got %d", got)
	}

	// Simulate out-of-band deletion on disk. In practice users often delete the
	// recording contents/metadata; the directory may temporarily remain.
	if err := os.Remove(filepath.Join(tmpDir, "info")); err != nil {
		t.Fatalf("Remove(info): %v", err)
	}

	// Second call is within cache expiry, so it should be a cache hit, but it must
	// prune the missing DiskPath entry.
	recs, err = svc.GetAllRecordings(ctx)
	if err != nil {
		t.Fatalf("GetAllRecordings(2): %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 recordings after out-of-band delete, got %d", len(recs))
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected no additional backend calls on cache hit, got %d", got)
	}
}
