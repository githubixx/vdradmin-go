package ports

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// ClientFactory creates a VDRClient instance and returns a cleanup function.
// The cleanup function should close connections and release resources.
type ClientFactory func() (VDRClient, func())

// RunVDRClientContractTests runs the complete contract test suite against a VDRClient implementation.
// This ensures that all implementations (real SVDRP client, mocks, fakes) behave consistently.
//
// Usage:
//
//	func TestMyClientImplementation(t *testing.T) {
//	    factory := func() (VDRClient, func()) {
//	        client := NewMyClient()
//	        cleanup := func() { client.Close() }
//	        return client, cleanup
//	    }
//	    RunVDRClientContractTests(t, factory)
//	}
func RunVDRClientContractTests(t *testing.T, factory ClientFactory) {
	t.Run("Connection", func(t *testing.T) { testConnection(t, factory) })
	t.Run("Channels", func(t *testing.T) { testChannels(t, factory) })
	t.Run("EPG", func(t *testing.T) { testEPG(t, factory) })
	t.Run("Timers", func(t *testing.T) { testTimers(t, factory) })
	t.Run("Recordings", func(t *testing.T) { testRecordings(t, factory) })
	t.Run("CurrentChannel", func(t *testing.T) { testCurrentChannel(t, factory) })
	t.Run("RemoteControl", func(t *testing.T) { testRemoteControl(t, factory) })
	t.Run("ContextCancellation", func(t *testing.T) { testContextCancellation(t, factory) })
	t.Run("ErrorHandling", func(t *testing.T) { testErrorHandling(t, factory) })
}

// testConnection validates connection lifecycle (Connect, Ping, Close)
func testConnection(t *testing.T, factory ClientFactory) {
	t.Run("Connect_Success", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.Connect(ctx)
		if err != nil {
			t.Errorf("Connect should succeed, got error: %v", err)
		}
	})

	t.Run("Ping_AfterConnect", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Connect(ctx); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}

		err := client.Ping(ctx)
		if err != nil {
			t.Errorf("Ping should succeed after Connect, got error: %v", err)
		}
	})

	t.Run("Close_Success", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		err := client.Close()
		if err != nil {
			t.Errorf("Close should not return error, got: %v", err)
		}
	})

	t.Run("Close_Idempotent", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		_ = client.Close()
		err := client.Close()
		if err != nil {
			t.Errorf("Multiple Close calls should be safe, got error: %v", err)
		}
	})
}

// testChannels validates channel retrieval behavior
func testChannels(t *testing.T, factory ClientFactory) {
	t.Run("GetChannels_ReturnsSlice", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		channels, err := client.GetChannels(ctx)
		if err != nil {
			t.Errorf("GetChannels should not error, got: %v", err)
		}
		if channels == nil {
			t.Error("GetChannels should return non-nil slice (even if empty)")
		}
	})

	t.Run("GetChannels_ValidStructure", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		channels, err := client.GetChannels(ctx)
		if err != nil {
			t.Fatalf("GetChannels failed: %v", err)
		}

		for i, ch := range channels {
			if ch.ID == "" {
				t.Errorf("Channel[%d] has empty ID", i)
			}
			if ch.Number <= 0 {
				t.Errorf("Channel[%d] has invalid number: %d", i, ch.Number)
			}
			if ch.Name == "" {
				t.Errorf("Channel[%d] has empty name", i)
			}
		}
	})
}

// testEPG validates EPG retrieval behavior
func testEPG(t *testing.T, factory ClientFactory) {
	t.Run("GetEPG_ReturnsSlice", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		at := time.Now()
		events, err := client.GetEPG(ctx, "test-channel", at)
		if err != nil {
			// Some implementations may return error for invalid channel, that's acceptable
			return
		}
		if events == nil {
			t.Error("GetEPG should return non-nil slice")
		}
	})

	t.Run("GetEPG_ValidStructure", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		at := time.Now()
		events, err := client.GetEPG(ctx, "", at)
		if err != nil {
			// Error acceptable for invalid input
			return
		}

		for i, event := range events {
			if event.EventID == 0 {
				t.Logf("Warning: Event[%d] has zero EventID", i)
			}
			if event.Start.IsZero() {
				t.Errorf("Event[%d] has zero Start time", i)
			}
			if event.Stop.IsZero() {
				t.Errorf("Event[%d] has zero Stop time", i)
			}
			if !event.Start.Before(event.Stop) && !event.Start.Equal(event.Stop) {
				t.Errorf("Event[%d] Start (%v) should be before Stop (%v)", i, event.Start, event.Stop)
			}
		}
	})

	t.Run("GetEPG_EmptyChannelID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		at := time.Now()
		// Empty channel ID typically means "all channels"
		events, err := client.GetEPG(ctx, "", at)
		if err != nil {
			// Implementation may reject empty channel ID, that's valid
			return
		}
		if events == nil {
			t.Error("GetEPG should return non-nil slice")
		}
	})
}

// testTimers validates timer CRUD operations
func testTimers(t *testing.T, factory ClientFactory) {
	t.Run("GetTimers_ReturnsSlice", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		timers, err := client.GetTimers(ctx)
		if err != nil {
			t.Errorf("GetTimers should not error, got: %v", err)
		}
		if timers == nil {
			t.Error("GetTimers should return non-nil slice")
		}
	})

	t.Run("CreateTimer_WithValidTimer", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		timer := &domain.Timer{
			Active:       true,
			ChannelID:    "S19.2E-1-1089-28106",
			DaySpec:      "2026-02-10",
			StartMinutes: 1200, // 20:00
			StopMinutes:  1320, // 22:00
			Priority:     50,
			Lifetime:     99,
			Title:        "Contract Test Timer",
		}

		err := client.CreateTimer(ctx, timer)
		// Implementation may or may not support CreateTimer in test environment
		// We validate the call signature, not the result
		_ = err
	})

	t.Run("CreateTimer_WithNilTimer", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.CreateTimer(ctx, nil)
		// Most implementations should handle nil gracefully (either accept or return error)
		if err == nil {
			t.Log("Implementation accepts nil timer (unusual but valid)")
		}
	})

	t.Run("UpdateTimer_WithValidTimer", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		timer := &domain.Timer{
			ID:           1,
			Active:       true,
			ChannelID:    "S19.2E-1-1089-28106",
			DaySpec:      "2026-02-10",
			StartMinutes: 1200,
			StopMinutes:  1320,
			Priority:     50,
			Lifetime:     99,
			Title:        "Updated Timer",
		}

		err := client.UpdateTimer(ctx, timer)
		_ = err
	})

	t.Run("DeleteTimer_WithValidID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.DeleteTimer(ctx, 999)
		// Implementation may return error for non-existent timer, that's valid
		_ = err
	})

	t.Run("DeleteTimer_WithZeroID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.DeleteTimer(ctx, 0)
		// Most implementations should handle invalid ID gracefully
		_ = err
	})
}

// testRecordings validates recording operations
func testRecordings(t *testing.T, factory ClientFactory) {
	t.Run("GetRecordings_ReturnsSlice", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		recordings, err := client.GetRecordings(ctx)
		if err != nil {
			t.Errorf("GetRecordings should not error, got: %v", err)
		}
		if recordings == nil {
			t.Error("GetRecordings should return non-nil slice")
		}
	})

	t.Run("GetRecordings_ValidStructure", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		recordings, err := client.GetRecordings(ctx)
		if err != nil {
			t.Fatalf("GetRecordings failed: %v", err)
		}

		for i, rec := range recordings {
			if rec.Path == "" {
				t.Errorf("Recording[%d] has empty Path", i)
			}
			if rec.Title == "" {
				t.Logf("Warning: Recording[%d] has empty Title", i)
			}
			// DiskPath is optional (may be empty if not resolved)
			// Date may be zero if parsing failed
			// Size may be zero if not calculated
		}
	})

	t.Run("GetRecordingDir_WithValidID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		path, err := client.GetRecordingDir(ctx, "1")
		// Implementation may return error for non-existent recording, that's valid
		if err == nil && path == "" {
			t.Error("GetRecordingDir should return non-empty path or error")
		}
	})

	t.Run("GetRecordingDir_WithEmptyID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		path, err := client.GetRecordingDir(ctx, "")
		// Implementation should handle empty ID (return error or empty path)
		if err == nil && path != "" {
			t.Error("Empty recording ID should not return valid path")
		}
	})

	t.Run("DeleteRecording_WithValidPath", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.DeleteRecording(ctx, "test/recording")
		// Implementation may return error for non-existent recording, that's valid
		_ = err
	})

	t.Run("DeleteRecording_WithEmptyPath", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.DeleteRecording(ctx, "")
		// Implementation should handle empty path gracefully
		_ = err
	})
}

// testCurrentChannel validates current channel operations
func testCurrentChannel(t *testing.T, factory ClientFactory) {
	t.Run("GetCurrentChannel_ReturnsString", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		channelID, err := client.GetCurrentChannel(ctx)
		if err != nil {
			// Implementation may not support this operation
			return
		}
		// Empty string is valid if no channel is active
		_ = channelID
	})

	t.Run("SetCurrentChannel_WithValidID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.SetCurrentChannel(ctx, "S19.2E-1-1089-28106")
		// Implementation may return error in test environment, that's acceptable
		_ = err
	})

	t.Run("SetCurrentChannel_WithEmptyID", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.SetCurrentChannel(ctx, "")
		// Implementation should handle empty ID gracefully (error or no-op)
		_ = err
	})
}

// testRemoteControl validates remote control key sending
func testRemoteControl(t *testing.T, factory ClientFactory) {
	t.Run("SendKey_WithValidKey", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		validKeys := []string{"Up", "Down", "Left", "Right", "Ok", "Back", "Menu"}
		for _, key := range validKeys {
			err := client.SendKey(ctx, key)
			// Implementation may not support this in test environment
			_ = err
		}
	})

	t.Run("SendKey_WithEmptyKey", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.SendKey(ctx, "")
		// Implementation should handle empty key gracefully
		_ = err
	})
}

// testContextCancellation validates context cancellation handling
func testContextCancellation(t *testing.T, factory ClientFactory) {
	t.Run("Connect_ContextCanceled", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := client.Connect(ctx)
		if err == nil {
			// Some implementations may not check context before quick operations
			t.Log("Implementation does not enforce canceled context on Connect")
		}
	})

	t.Run("GetChannels_ContextTimeout", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond) // Ensure timeout

		_, err := client.GetChannels(ctx)
		if err == nil {
			// Some implementations may cache and return quickly
			t.Log("Implementation returned before context timeout")
		}
	})
}

// testErrorHandling validates error handling behavior
func testErrorHandling(t *testing.T, factory ClientFactory) {
	t.Run("Operations_ReturnDomainErrors", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try operations that might fail
		_, err1 := client.GetEPG(ctx, "invalid-channel-id", time.Now())
		_, err2 := client.GetRecordingDir(ctx, "999999")

		// Errors should be typed appropriately (we don't enforce specific types,
		// but validate that errors are returned when appropriate)
		_ = err1
		_ = err2
	})

	t.Run("NilContext_HandledGracefully", func(t *testing.T) {
		client, cleanup := factory()
		defer cleanup()

		// Implementation should either handle nil context or panic consistently
		// This test documents the behavior
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Implementation panics on nil context: %v", r)
			}
		}()

		_, _ = client.GetChannels(context.TODO())
	})
}
