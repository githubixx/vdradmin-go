package services

import (
	"context"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// mockVDRClient is a flexible test double for ports.VDRClient.
// Add function fields as tests grow; unconfigured methods return zero values.
//
// Keeping this in one place avoids copy/paste and makes future test expansion easier.
type mockVDRClient struct {
	ConnectFunc          func(ctx context.Context) error
	CloseFunc            func() error
	PingFunc             func(ctx context.Context) error
	GetChannelsFunc      func(ctx context.Context) ([]domain.Channel, error)
	GetEPGFunc           func(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error)
	GetTimersFunc        func(ctx context.Context) ([]domain.Timer, error)
	CreateTimerFunc      func(ctx context.Context, timer *domain.Timer) error
	UpdateTimerFunc      func(ctx context.Context, timer *domain.Timer) error
	DeleteTimerFunc      func(ctx context.Context, timerID int) error
	GetRecordingsFunc    func(ctx context.Context) ([]domain.Recording, error)
	DeleteRecordingFunc  func(ctx context.Context, path string) error
	GetCurrentChannelFunc func(ctx context.Context) (string, error)
	SetCurrentChannelFunc func(ctx context.Context, channelID string) error
	SendKeyFunc          func(ctx context.Context, key string) error
}

var _ ports.VDRClient = (*mockVDRClient)(nil)

func (m *mockVDRClient) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	return nil
}

func (m *mockVDRClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *mockVDRClient) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *mockVDRClient) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	if m.GetChannelsFunc != nil {
		return m.GetChannelsFunc(ctx)
	}
	return nil, nil
}

func (m *mockVDRClient) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if m.GetEPGFunc != nil {
		return m.GetEPGFunc(ctx, channelID, at)
	}
	return nil, nil
}

func (m *mockVDRClient) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	if m.GetTimersFunc != nil {
		return m.GetTimersFunc(ctx)
	}
	return nil, nil
}

func (m *mockVDRClient) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	if m.CreateTimerFunc != nil {
		return m.CreateTimerFunc(ctx, timer)
	}
	return nil
}

func (m *mockVDRClient) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	if m.UpdateTimerFunc != nil {
		return m.UpdateTimerFunc(ctx, timer)
	}
	return nil
}

func (m *mockVDRClient) DeleteTimer(ctx context.Context, timerID int) error {
	if m.DeleteTimerFunc != nil {
		return m.DeleteTimerFunc(ctx, timerID)
	}
	return nil
}

func (m *mockVDRClient) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	if m.GetRecordingsFunc != nil {
		return m.GetRecordingsFunc(ctx)
	}
	return nil, nil
}

func (m *mockVDRClient) DeleteRecording(ctx context.Context, path string) error {
	if m.DeleteRecordingFunc != nil {
		return m.DeleteRecordingFunc(ctx, path)
	}
	return nil
}

func (m *mockVDRClient) GetCurrentChannel(ctx context.Context) (string, error) {
	if m.GetCurrentChannelFunc != nil {
		return m.GetCurrentChannelFunc(ctx)
	}
	return "", nil
}

func (m *mockVDRClient) SetCurrentChannel(ctx context.Context, channelID string) error {
	if m.SetCurrentChannelFunc != nil {
		return m.SetCurrentChannelFunc(ctx, channelID)
	}
	return nil
}

func (m *mockVDRClient) SendKey(ctx context.Context, key string) error {
	if m.SendKeyFunc != nil {
		return m.SendKeyFunc(ctx, key)
	}
	return nil
}
