package ports

import (
	"context"
	"sync"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// MockVDRClient is a flexible test double for VDRClient with function field customization.
// This is the canonical mock implementation used across all tests.
//
// Usage with function fields (maximum flexibility):
//
//	mock := &ports.MockVDRClient{
//	    GetChannelsFunc: func(ctx context.Context) ([]domain.Channel, error) {
//	        return []domain.Channel{{ID: "test", Number: 1, Name: "Test"}}, nil
//	    },
//	}
//
// Usage with builder pattern (convenience):
//
//	mock := ports.NewMockVDRClient().
//	    WithChannels([]domain.Channel{{ID: "test", Number: 1, Name: "Test"}}).
//	    WithTimers([]domain.Timer{{ID: 1, Title: "Test"}})
type MockVDRClient struct {
	// Function fields for custom behavior
	ConnectFunc           func(ctx context.Context) error
	CloseFunc             func() error
	PingFunc              func(ctx context.Context) error
	GetChannelsFunc       func(ctx context.Context) ([]domain.Channel, error)
	GetEPGFunc            func(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error)
	GetTimersFunc         func(ctx context.Context) ([]domain.Timer, error)
	CreateTimerFunc       func(ctx context.Context, timer *domain.Timer) error
	UpdateTimerFunc       func(ctx context.Context, timer *domain.Timer) error
	DeleteTimerFunc       func(ctx context.Context, timerID int) error
	GetRecordingsFunc     func(ctx context.Context) ([]domain.Recording, error)
	GetRecordingDirFunc   func(ctx context.Context, recordingID string) (string, error)
	DeleteRecordingFunc   func(ctx context.Context, path string) error
	GetCurrentChannelFunc func(ctx context.Context) (string, error)
	SetCurrentChannelFunc func(ctx context.Context, channelID string) error
	SendKeyFunc           func(ctx context.Context, key string) error

	// Data fields for builder pattern
	mu             sync.RWMutex
	channels       []domain.Channel
	epgEvents      []domain.EPGEvent
	timers         []domain.Timer
	recordings     []domain.Recording
	currentChannel string
}

var _ VDRClient = (*MockVDRClient)(nil)

// NewMockVDRClient creates a new mock with default behavior.
// Use builder methods to configure data or set function fields directly for custom behavior.
func NewMockVDRClient() *MockVDRClient {
	return &MockVDRClient{
		channels:   []domain.Channel{},
		epgEvents:  []domain.EPGEvent{},
		timers:     []domain.Timer{},
		recordings: []domain.Recording{},
	}
}

// WithChannels sets the channels returned by GetChannels.
func (m *MockVDRClient) WithChannels(channels []domain.Channel) *MockVDRClient {
	m.channels = channels
	return m
}

// WithEPGEvents sets the EPG events returned by GetEPG.
func (m *MockVDRClient) WithEPGEvents(events []domain.EPGEvent) *MockVDRClient {
	m.epgEvents = events
	return m
}

// WithTimers sets the timers returned by GetTimers.
func (m *MockVDRClient) WithTimers(timers []domain.Timer) *MockVDRClient {
	m.timers = timers
	return m
}

// WithRecordings sets the recordings returned by GetRecordings.
func (m *MockVDRClient) WithRecordings(recordings []domain.Recording) *MockVDRClient {
	m.recordings = recordings
	return m
}

// WithCurrentChannel sets the current channel ID.
func (m *MockVDRClient) WithCurrentChannel(channelID string) *MockVDRClient {
	m.currentChannel = channelID
	return m
}

// Implementation of VDRClient interface

func (m *MockVDRClient) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	return nil
}

func (m *MockVDRClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockVDRClient) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *MockVDRClient) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	if m.GetChannelsFunc != nil {
		return m.GetChannelsFunc(ctx)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels, nil
}

func (m *MockVDRClient) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if m.GetEPGFunc != nil {
		return m.GetEPGFunc(ctx, channelID, at)
	}
	return m.epgEvents, nil
}

func (m *MockVDRClient) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	if m.GetTimersFunc != nil {
		return m.GetTimersFunc(ctx)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.timers, nil
}

func (m *MockVDRClient) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	if m.CreateTimerFunc != nil {
		return m.CreateTimerFunc(ctx, timer)
	}
	if timer == nil {
		return domain.ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Assign ID if not set
	if timer.ID == 0 {
		timer.ID = len(m.timers) + 1
	}
	m.timers = append(m.timers, *timer)
	return nil
}

func (m *MockVDRClient) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	if m.UpdateTimerFunc != nil {
		return m.UpdateTimerFunc(ctx, timer)
	}
	if timer == nil {
		return domain.ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.timers {
		if t.ID == timer.ID {
			m.timers[i] = *timer
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *MockVDRClient) DeleteTimer(ctx context.Context, timerID int) error {
	if m.DeleteTimerFunc != nil {
		return m.DeleteTimerFunc(ctx, timerID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.timers {
		if t.ID == timerID {
			m.timers = append(m.timers[:i], m.timers[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *MockVDRClient) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	if m.GetRecordingsFunc != nil {
		return m.GetRecordingsFunc(ctx)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recordings, nil
}

func (m *MockVDRClient) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	if m.GetRecordingDirFunc != nil {
		return m.GetRecordingDirFunc(ctx, recordingID)
	}
	if recordingID == "" {
		return "", domain.ErrInvalidInput
	}
	return "/video/recordings/" + recordingID, nil
}

func (m *MockVDRClient) DeleteRecording(ctx context.Context, path string) error {
	if m.DeleteRecordingFunc != nil {
		return m.DeleteRecordingFunc(ctx, path)
	}
	if path == "" {
		return domain.ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.recordings {
		if r.Path == path {
			m.recordings = append(m.recordings[:i], m.recordings[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *MockVDRClient) GetCurrentChannel(ctx context.Context) (string, error) {
	if m.GetCurrentChannelFunc != nil {
		return m.GetCurrentChannelFunc(ctx)
	}
	return m.currentChannel, nil
}

func (m *MockVDRClient) SetCurrentChannel(ctx context.Context, channelID string) error {
	if m.SetCurrentChannelFunc != nil {
		return m.SetCurrentChannelFunc(ctx, channelID)
	}
	if channelID == "" {
		return domain.ErrInvalidInput
	}
	m.currentChannel = channelID
	return nil
}

func (m *MockVDRClient) SendKey(ctx context.Context, key string) error {
	if m.SendKeyFunc != nil {
		return m.SendKeyFunc(ctx, key)
	}
	if key == "" {
		return domain.ErrInvalidInput
	}
	return nil
}
