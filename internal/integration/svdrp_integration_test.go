//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSVDRP_RealProtocol_ChannelListRetrieval validates the SVDRP client
// can communicate with a real SVDRP server and parse channel data correctly.
func TestSVDRP_RealProtocol_ChannelListRetrieval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	// Test channel retrieval
	channels, err := client.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}

	if len(channels) == 0 {
		t.Fatal("expected at least one channel, got none")
	}

	// Verify channel structure
	ch := channels[0]
	if ch.ID == "" {
		t.Error("channel ID should not be empty")
	}
	if ch.Number == 0 {
		t.Error("channel number should not be zero")
	}
	if ch.Name == "" {
		t.Error("channel name should not be empty")
	}

	t.Logf("Retrieved %d channels from SVDRP stub", len(channels))
}

// TestSVDRP_RealProtocol_TimerCRUD validates complete timer lifecycle:
// create, read, update, delete through real SVDRP protocol.
func TestSVDRP_RealProtocol_TimerCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	// Get initial timers
	initialTimers, err := client.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers (initial): %v", err)
	}
	initialCount := len(initialTimers)
	t.Logf("Initial timer count: %d", initialCount)

	// Verify initial timers have expected structure
	if initialCount > 0 {
		tm := initialTimers[0]
		if tm.ID == 0 {
			t.Error("timer ID should not be zero")
		}
		if tm.ChannelID == "" {
			t.Error("timer channelID should not be empty")
		}
		if tm.Start.IsZero() {
			t.Error("timer start time should be set")
		}
		if tm.Stop.IsZero() {
			t.Error("timer stop time should be set")
		}
		t.Logf("Sample timer: ID=%d, Channel=%s, Title=%s, Start=%s",
			tm.ID, tm.ChannelID, tm.Title, tm.Start.Format(time.RFC3339))
	}

	// Create new timer
	newTimer := &domain.Timer{
		Active:    true,
		ChannelID: "C-1-1-10",
		Title:     "Integration Test Show",
		Start:     time.Now().Add(1 * time.Hour),
		Stop:      time.Now().Add(2 * time.Hour),
		Priority:  50,
		Lifetime:  99,
	}

	if err := client.CreateTimer(ctx, newTimer); err != nil {
		t.Fatalf("CreateTimer: %v", err)
	}
	t.Log("Timer created successfully")

	// Verify timer was created
	timersAfterCreate, err := client.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers (after create): %v", err)
	}
	if len(timersAfterCreate) != initialCount+1 {
		t.Errorf("expected %d timers after create, got %d", initialCount+1, len(timersAfterCreate))
	}

	// Find the created timer (stub assigns sequential IDs)
	var createdTimer *domain.Timer
	for i := range timersAfterCreate {
		if timersAfterCreate[i].Title == "Integration Test Show" {
			createdTimer = &timersAfterCreate[i]
			break
		}
	}
	if createdTimer == nil {
		t.Fatal("created timer not found in timer list")
	}
	t.Logf("Found created timer with ID=%d", createdTimer.ID)

	// Update timer
	createdTimer.Title = "Updated Integration Test"
	createdTimer.Priority = 75
	if err := client.UpdateTimer(ctx, createdTimer); err != nil {
		t.Fatalf("UpdateTimer: %v", err)
	}
	t.Log("Timer updated successfully")

	// Verify update
	timersAfterUpdate, err := client.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers (after update): %v", err)
	}
	var updatedTimer *domain.Timer
	for i := range timersAfterUpdate {
		if timersAfterUpdate[i].ID == createdTimer.ID {
			updatedTimer = &timersAfterUpdate[i]
			break
		}
	}
	if updatedTimer == nil {
		t.Fatal("updated timer not found")
	}
	if updatedTimer.Title != "Updated Integration Test" {
		t.Errorf("expected title 'Updated Integration Test', got %q", updatedTimer.Title)
	}

	// Delete timer
	if err := client.DeleteTimer(ctx, createdTimer.ID); err != nil {
		t.Fatalf("DeleteTimer: %v", err)
	}
	t.Log("Timer deleted successfully")

	// Verify deletion
	timersAfterDelete, err := client.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers (after delete): %v", err)
	}
	if len(timersAfterDelete) != initialCount {
		t.Errorf("expected %d timers after delete, got %d", initialCount, len(timersAfterDelete))
	}
	for _, tm := range timersAfterDelete {
		if tm.ID == createdTimer.ID {
			t.Error("deleted timer still present in timer list")
		}
	}
}

// TestSVDRP_RealProtocol_ConnectionResilience tests that the SVDRP client
// can handle connection errors and recover gracefully.
func TestSVDRP_RealProtocol_ConnectionResilience(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)

	// Verify initial connection works
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("initial Ping failed: %v", err)
	}

	// Close connection explicitly
	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Client should reconnect automatically on next operation
	channels, err := client.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels after reconnect: %v", err)
	}
	if len(channels) == 0 {
		t.Error("expected channels after reconnect, got none")
	}

	// Verify Ping still works
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping after reconnect: %v", err)
	}

	t.Log("Connection resilience validated - client reconnected successfully")
}

// TestSVDRP_RealProtocol_TimeoutHandling verifies the client respects
// context timeouts and doesn't hang indefinitely.
func TestSVDRP_RealProtocol_TimeoutHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	// Test with very short timeout
	shortCtx, shortCancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer shortCancel()

	// Operation should fail quickly due to timeout
	_, err := client.GetChannels(shortCtx)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	t.Logf("Timeout handled correctly: %v", err)

	// Verify client still works after timeout
	normalCtx, normalCancel := context.WithTimeout(ctx, 5*time.Second)
	defer normalCancel()

	channels, err := client.GetChannels(normalCtx)
	if err != nil {
		t.Fatalf("GetChannels after timeout: %v", err)
	}
	if len(channels) == 0 {
		t.Error("expected channels after timeout recovery")
	}
}

// TestSVDRP_RealProtocol_ConcurrentOperations validates that multiple
// goroutines can safely use the client concurrently.
func TestSVDRP_RealProtocol_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	// Run multiple operations concurrently
	const concurrency = 10
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			opCtx, opCancel := context.WithTimeout(ctx, 10*time.Second)
			defer opCancel()

			// Mix different operations
			switch id % 3 {
			case 0:
				_, err := client.GetChannels(opCtx)
				errChan <- err
			case 1:
				_, err := client.GetTimers(opCtx)
				errChan <- err
			case 2:
				err := client.Ping(opCtx)
				errChan <- err
			}
		}(i)
	}

	// Collect results
	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("concurrent operation %d failed: %v", i, err)
		}
	}

	t.Log("Concurrent operations completed successfully")
}

// TestSVDRP_RealProtocol_TimerCollisionDetection validates that the stub
// provides timer data with overlaps for testing conflict detection.
func TestSVDRP_RealProtocol_TimerCollisionDetection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	timers, err := client.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers: %v", err)
	}

	if len(timers) < 2 {
		t.Fatal("expected at least 2 timers to test collision detection")
	}

	// Check for overlapping timers (stub should provide these)
	var overlaps int
	for i := 0; i < len(timers); i++ {
		for j := i + 1; j < len(timers); j++ {
			// Check if timers overlap
			if timers[i].Start.Before(timers[j].Stop) && timers[j].Start.Before(timers[i].Stop) {
				overlaps++
				t.Logf("Found overlap: Timer %d (%s) and Timer %d (%s)",
					timers[i].ID, timers[i].Title, timers[j].ID, timers[j].Title)
			}
		}
	}

	if overlaps == 0 {
		t.Error("expected overlapping timers for collision detection testing")
	}
}

// TestSVDRP_RealProtocol_ChannelIDParsing validates that various channel
// ID formats are parsed correctly by the SVDRP client.
func TestSVDRP_RealProtocol_ChannelIDParsing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := startSVDRPStub(t, ctx)
	defer client.Close()

	channels, err := client.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}

	// Channel IDs should follow format like "C-1-1-10" or "S19.2E-1-100-10"
	for _, ch := range channels {
		if ch.ID == "" {
			t.Error("channel has empty ID")
			continue
		}

		// Should contain hyphens
		if !contains(ch.ID, "-") {
			t.Errorf("channel ID %q has unexpected format (no hyphens)", ch.ID)
		}

		t.Logf("Channel %d: ID=%s, Name=%s", ch.Number, ch.ID, ch.Name)
	}
}

// startSVDRPStub starts the SVDRP stub container and returns a connected client.
func startSVDRPStub(t *testing.T, ctx context.Context) *svdrp.Client {
	t.Helper()

	repoRoot := mustRepoRoot(t)

	svdrpContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    filepath.Join(repoRoot, "test/integration/svdrpstub"),
				Dockerfile: "Dockerfile",
			},
			ExposedPorts: []string{"6419/tcp"},
			WaitingFor:   wait.ForListeningPort("6419/tcp").WithStartupTimeout(45 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start svdrp container: %v", err)
	}
	t.Cleanup(func() { _ = svdrpContainer.Terminate(ctx) })

	// Get mapped port
	mappedPort, err := svdrpContainer.MappedPort(ctx, "6419/tcp")
	if err != nil {
		t.Fatalf("get mapped port: %v", err)
	}

	host, err := svdrpContainer.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}

	port := mappedPort.Int()
	t.Logf("SVDRP stub running at %s:%d", host, port)

	// Create and connect client
	client := svdrp.NewClient(host, port, 5*time.Second)

	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connectCancel()

	if err := client.Connect(connectCtx); err != nil {
		t.Fatalf("connect to SVDRP: %v", err)
	}

	return client
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
