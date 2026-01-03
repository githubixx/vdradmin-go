package http

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

type timerCreateVDRMock struct {
	mu sync.Mutex

	events []domain.EPGEvent

	created []*domain.Timer
}

func (m *timerCreateVDRMock) Connect(ctx context.Context) error { return nil }
func (m *timerCreateVDRMock) Close() error                     { return nil }
func (m *timerCreateVDRMock) Ping(ctx context.Context) error    { return nil }
func (m *timerCreateVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return nil, nil
}

func (m *timerCreateVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.EPGEvent, len(m.events))
	copy(out, m.events)
	return out, nil
}

func (m *timerCreateVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) { return nil, nil }

func (m *timerCreateVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *timer
	m.created = append(m.created, &cp)
	return nil
}

func (m *timerCreateVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *timerCreateVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }
func (m *timerCreateVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *timerCreateVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *timerCreateVDRMock) GetCurrentChannel(ctx context.Context) (string, error) { return "", nil }
func (m *timerCreateVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *timerCreateVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestTimerCreate_UsesConfiguredDefaultMargins(t *testing.T) {
	start := time.Date(2026, 1, 3, 20, 0, 0, 0, time.Local)
	stop := time.Date(2026, 1, 3, 21, 0, 0, 0, time.Local)

	mock := &timerCreateVDRMock{events: []domain.EPGEvent{{
		EventID:   123,
		ChannelID: "C-1-2-3",
		Title:     "Show",
		Start:     start,
		Stop:      stop,
	}}}

	epgSvc := services.NewEPGService(mock, 0)
	timerSvc := services.NewTimerService(mock)
	recSvc := services.NewRecordingService(mock, 0)
	autoSvc := services.NewAutoTimerService(mock, timerSvc, epgSvc)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	tpl := template.New("test")

	h := NewHandler(logger, tpl, epgSvc, timerSvc, recSvc, autoSvc)
	h.SetConfig(&config.Config{Timer: config.TimerConfig{
		DefaultPriority:    50,
		DefaultLifetime:    99,
		DefaultMarginStart: 7,
		DefaultMarginEnd:   11,
	}}, "")

	form := url.Values{}
	form.Set("event_id", "123")
	form.Set("channel", "C-1-2-3")

	req := httptest.NewRequest(http.MethodPost, "/timers/create", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.TimerCreate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected %d, got %d", http.StatusSeeOther, resp.StatusCode)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.created) != 1 {
		t.Fatalf("expected 1 created timer, got %d", len(mock.created))
	}
	created := mock.created[0]

	expectedStart := start.Add(-7 * time.Minute)
	expectedStop := stop.Add(11 * time.Minute)
	if !created.Start.Equal(expectedStart) {
		t.Fatalf("expected Start %s, got %s", expectedStart.Format(time.RFC3339), created.Start.Format(time.RFC3339))
	}
	if !created.Stop.Equal(expectedStop) {
		t.Fatalf("expected Stop %s, got %s", expectedStop.Format(time.RFC3339), created.Stop.Format(time.RFC3339))
	}
}
