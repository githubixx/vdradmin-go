package http

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

type timersTimelineVDRMock struct {
	channels []domain.Channel
	timers   []domain.Timer
}

func (m *timersTimelineVDRMock) Connect(ctx context.Context) error { return nil }
func (m *timersTimelineVDRMock) Close() error                      { return nil }
func (m *timersTimelineVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *timersTimelineVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *timersTimelineVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return nil, nil
}

func (m *timersTimelineVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return m.timers, nil
}

func (m *timersTimelineVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timersTimelineVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timersTimelineVDRMock) DeleteTimer(ctx context.Context, timerID int) error { return nil }

func (m *timersTimelineVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *timersTimelineVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *timersTimelineVDRMock) GetCurrentChannel(ctx context.Context) (string, error) {
	return "", nil
}
func (m *timersTimelineVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *timersTimelineVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestTimerList_RendersTimeline(t *testing.T) {
	loc := time.Local
	day := time.Date(2026, 1, 6, 0, 0, 0, 0, loc)

	ch1 := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}
	ch2 := domain.Channel{ID: "S19.2E-1-200-20", Number: 2, Name: "ZDF HD"}
	ch3 := domain.Channel{ID: "S19.2E-1-300-30", Number: 3, Name: "zdf_neo HD"}

	t1 := domain.Timer{ID: 1, Active: true, ChannelID: ch1.ID, Title: "Show A", Start: day.Add(1 * time.Hour), Stop: day.Add(2 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: ch2.ID, Title: "Show B", Start: day.Add(90 * time.Minute), Stop: day.Add(150 * time.Minute)}
	t3 := domain.Timer{ID: 3, Active: true, ChannelID: ch3.ID, Title: "Show C", Start: day.Add(5 * time.Hour), Stop: day.Add(6 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch1, ch2, ch3},
		timers:   []domain.Timer{t1, t2, t3},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		epqService,
		timerService,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	req := httptest.NewRequest(http.MethodGet, "/timers?day=2026-01-06", nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerList(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	if !strings.Contains(body, "timer-timeline") {
		t.Fatalf("expected timeline container")
	}
	if !strings.Contains(body, "timeline-hours") {
		t.Fatalf("expected hours header")
	}
	if !strings.Contains(body, "SWR BW HD") || !strings.Contains(body, "ZDF HD") {
		t.Fatalf("expected channel names in timeline")
	}
	if !strings.Contains(body, "timeline-block collision") {
		t.Fatalf("expected collision (yellow) block")
	}
	if !strings.Contains(body, "timeline-block ok") {
		t.Fatalf("expected ok (green) block")
	}
}
