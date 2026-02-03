package ports

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// TestMockVDRClient_ContractCompliance verifies that the canonical mock implementation
// satisfies the VDRClient contract test suite.
func TestMockVDRClient_ContractCompliance(t *testing.T) {
	factory := func() (VDRClient, func()) {
		mock := NewMockVDRClient().
			WithChannels([]domain.Channel{
				{ID: "S19.2E-1-1089-28106", Number: 1, Name: "Das Erste HD"},
				{ID: "S19.2E-1-1089-28107", Number: 2, Name: "ZDF HD"},
			}).
			WithRecordings([]domain.Recording{
				{Path: "test/recording1", Title: "Test Recording 1", Date: time.Now()},
				{Path: "test/recording2", Title: "Test Recording 2", Date: time.Now()},
			}).
			WithTimers([]domain.Timer{
				{ID: 1, Active: true, Title: "Test Timer", ChannelID: "S19.2E-1-1089-28106"},
			}).
			WithCurrentChannel("S19.2E-1-1089-28106")

		cleanup := func() {}
		return mock, cleanup
	}

	RunVDRClientContractTests(t, factory)
}

// TestMockVDRClient_BuilderPattern demonstrates using the builder pattern
func TestMockVDRClient_BuilderPattern(t *testing.T) {
	t.Run("WithChannels", func(t *testing.T) {
		mock := NewMockVDRClient().WithChannels([]domain.Channel{
			{ID: "test-1", Number: 1, Name: "Test Channel"},
		})

		channels, err := mock.GetChannels(context.Background())
		if err != nil {
			t.Fatalf("GetChannels failed: %v", err)
		}
		if len(channels) != 1 {
			t.Errorf("Expected 1 channel, got %d", len(channels))
		}
		if channels[0].Name != "Test Channel" {
			t.Errorf("Expected 'Test Channel', got %s", channels[0].Name)
		}
	})

	t.Run("WithTimers", func(t *testing.T) {
		mock := NewMockVDRClient().WithTimers([]domain.Timer{
			{ID: 1, Title: "Test Timer"},
		})

		timers, err := mock.GetTimers(context.Background())
		if err != nil {
			t.Fatalf("GetTimers failed: %v", err)
		}
		if len(timers) != 1 {
			t.Errorf("Expected 1 timer, got %d", len(timers))
		}
	})

	t.Run("ChainedBuilders", func(t *testing.T) {
		mock := NewMockVDRClient().
			WithChannels([]domain.Channel{{ID: "1", Number: 1, Name: "Ch1"}}).
			WithTimers([]domain.Timer{{ID: 1, Title: "Timer1"}}).
			WithCurrentChannel("1")

		channels, _ := mock.GetChannels(context.Background())
		timers, _ := mock.GetTimers(context.Background())
		currentCh, _ := mock.GetCurrentChannel(context.Background())

		if len(channels) != 1 || len(timers) != 1 || currentCh != "1" {
			t.Error("Chained builders should set all fields")
		}
	})
}

// TestMockVDRClient_FunctionFields demonstrates using function field customization
func TestMockVDRClient_FunctionFields(t *testing.T) {
	t.Run("CustomGetChannels", func(t *testing.T) {
		mock := &MockVDRClient{
			GetChannelsFunc: func(ctx context.Context) ([]domain.Channel, error) {
				return []domain.Channel{
					{ID: "custom", Number: 99, Name: "Custom Channel"},
				}, nil
			},
		}

		channels, err := mock.GetChannels(context.Background())
		if err != nil {
			t.Fatalf("GetChannels failed: %v", err)
		}
		if channels[0].Number != 99 {
			t.Errorf("Expected custom number 99, got %d", channels[0].Number)
		}
	})

	t.Run("CustomErrorBehavior", func(t *testing.T) {
		mock := &MockVDRClient{
			GetTimersFunc: func(ctx context.Context) ([]domain.Timer, error) {
				return nil, domain.ErrConnection
			},
		}

		_, err := mock.GetTimers(context.Background())
		if err != domain.ErrConnection {
			t.Errorf("Expected ErrConnection, got %v", err)
		}
	})
}
