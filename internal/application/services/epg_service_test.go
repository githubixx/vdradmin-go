package services

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestEPGService_GetEPG_UsesCache(t *testing.T) {
	var calls int32
	client := &mockVDRClient{
		GetEPGFunc: func(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
			atomic.AddInt32(&calls, 1)
			return []domain.EPGEvent{{EventID: 1, ChannelID: channelID, Title: "X", Start: time.Unix(10, 0), Stop: time.Unix(20, 0)}}, nil
		},
	}

	svc := NewEPGService(client, 1*time.Minute)
	ctx := context.Background()

	_, err := svc.GetEPG(ctx, "C-1-2-3", time.Unix(123, 0))
	if err != nil {
		t.Fatalf("GetEPG(1): %v", err)
	}
	_, err = svc.GetEPG(ctx, "C-1-2-3", time.Unix(123, 0))
	if err != nil {
		t.Fatalf("GetEPG(2): %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 backend call, got %d", got)
	}
}

func TestEPGService_GetChannels_UsesCache(t *testing.T) {
	var calls int32
	client := &mockVDRClient{
		GetChannelsFunc: func(ctx context.Context) ([]domain.Channel, error) {
			atomic.AddInt32(&calls, 1)
			return []domain.Channel{{ID: "C-1-2-3", Number: 1, Name: "Ch"}}, nil
		},
	}

	svc := NewEPGService(client, 1*time.Minute)
	ctx := context.Background()

	_, err := svc.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels(1): %v", err)
	}
	_, err = svc.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels(2): %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 backend call, got %d", got)
	}
}

func TestEPGService_WantedChannels_FiltersChannels(t *testing.T) {
	client := &mockVDRClient{
		GetChannelsFunc: func(ctx context.Context) ([]domain.Channel, error) {
			return []domain.Channel{
				{ID: "C-1-2-3", Number: 1, Name: "One"},
				{ID: "C-9-9-9", Number: 2, Name: "Two"},
			}, nil
		},
	}

	svc := NewEPGService(client, 1*time.Minute)
	svc.SetWantedChannels([]string{"C-9-9-9"})

	chs, err := svc.GetChannels(context.Background())
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}
	if chs[0].ID != "C-9-9-9" {
		t.Fatalf("expected C-9-9-9, got %q", chs[0].ID)
	}
}

func TestEPGService_WantedChannels_FiltersAllEPG(t *testing.T) {
	client := &mockVDRClient{
		GetEPGFunc: func(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
			return []domain.EPGEvent{
				{EventID: 1, ChannelID: "C-1-2-3", Title: "A", Start: time.Unix(10, 0), Stop: time.Unix(20, 0)},
				{EventID: 2, ChannelID: "C-9-9-9", Title: "B", Start: time.Unix(11, 0), Stop: time.Unix(21, 0)},
			}, nil
		},
	}

	svc := NewEPGService(client, 1*time.Minute)
	svc.SetWantedChannels([]string{"C-9-9-9"})

	events, err := svc.GetEPG(context.Background(), "", time.Time{})
	if err != nil {
		t.Fatalf("GetEPG(all): %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ChannelID != "C-9-9-9" {
		t.Fatalf("expected event on C-9-9-9, got %q", events[0].ChannelID)
	}
}

func TestEPGService_WantedChannels_SkipsBackendForUnwantedChannel(t *testing.T) {
	var calls int32
	client := &mockVDRClient{
		GetEPGFunc: func(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
			atomic.AddInt32(&calls, 1)
			return []domain.EPGEvent{{EventID: 1, ChannelID: channelID, Title: "X", Start: time.Unix(10, 0), Stop: time.Unix(20, 0)}}, nil
		},
	}

	svc := NewEPGService(client, 1*time.Minute)
	svc.SetWantedChannels([]string{"C-1-2-3"})

	events, err := svc.GetEPG(context.Background(), "C-9-9-9", time.Time{})
	if err != nil {
		t.Fatalf("GetEPG(unwanted): %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected 0 backend calls, got %d", got)
	}
}
