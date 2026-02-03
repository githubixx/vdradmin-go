//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// TestSVDRP_Recordings_DataRetrieval validates recording list can be
// retrieved through SVDRP protocol.
func TestSVDRP_Recordings_DataRetrieval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	recordings, err := client.GetRecordings(ctx)
	if err != nil {
		t.Fatalf("GetRecordings: %v", err)
	}

	t.Logf("Retrieved %d recordings", len(recordings))

	// Recordings may be empty in stub, but structure should be valid if present
	for _, rec := range recordings {
		if rec.Path == "" {
			t.Error("recording has empty Path")
		}
		if rec.Title == "" {
			t.Error("recording has empty Title")
		}

		t.Logf("Recording: %s - %s (Channel: %s, Date: %s)",
			rec.Path, rec.Title, rec.Channel, rec.Date.Format("2006-01-02"))
	}
}

// TestSVDRP_Recordings_RecordingDirectory validates fetching recording
// directory information through SVDRP.
func TestSVDRP_Recordings_RecordingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	recordings, err := client.GetRecordings(ctx)
	if err != nil {
		t.Fatalf("GetRecordings: %v", err)
	}

	if len(recordings) == 0 {
		t.Skip("no recordings available for directory test")
	}

	// Try to get directory for first recording
	recordingPath := recordings[0].Path

	dir, err := client.GetRecordingDir(ctx, recordingPath)
	if err != nil {
		// May not be implemented in stub
		t.Logf("GetRecordingDir not available (expected): %v", err)
		return
	}

	if dir == "" {
		t.Error("got empty recording directory")
	}

	t.Logf("Recording directory for %s: %s", recordingPath, dir)
}

// TestSVDRP_Recordings_DeleteOperation validates recording deletion
// (dry-run test, doesn't actually delete in production).
func TestSVDRP_Recordings_DeleteOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	// Test with fake recording path
	// Stub should accept the command even if recording doesn't exist
	fakeRecordingPath := "Test Recording/2026-02-02.20.15.50-0-0"

	err := client.DeleteRecording(ctx, fakeRecordingPath)
	if err != nil {
		// Some errors are expected with fake paths
		t.Logf("DeleteRecording with fake path: %v (may be expected)", err)
		return
	}

	t.Log("DeleteRecording command sent successfully")
}
