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
