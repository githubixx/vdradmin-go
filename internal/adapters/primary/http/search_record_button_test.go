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
)

type searchVDRMock struct {
	channels []domain.Channel
	epg      []domain.EPGEvent
	timers   []domain.Timer
}

func (m *searchVDRMock) Connect(ctx context.Context) error { return nil }
func (m *searchVDRMock) Close() error                      { return nil }
func (m *searchVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *searchVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *searchVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return m.epg, nil
}

func (m *searchVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return m.timers, nil
}

func (m *searchVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *searchVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *searchVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (m *searchVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}

func (m *searchVDRMock) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}
func (m *searchVDRMock) DeleteRecording(ctx context.Context, path string) error        { return nil }
func (m *searchVDRMock) GetCurrentChannel(ctx context.Context) (string, error)         { return "", nil }
func (m *searchVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error { return nil }
func (m *searchVDRMock) SendKey(ctx context.Context, key string) error                 { return nil }

func TestSearch_DisablesRecordWhenTimerExists(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(48 * time.Hour)

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}

	scheduled := domain.EPGEvent{
		EventID:       100,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Show A",
		Start:         day.Add(9 * time.Hour),
		Stop:          day.Add(10 * time.Hour),
	}
	other := domain.EPGEvent{
		EventID:       101,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Show B",
		Start:         day.Add(11 * time.Hour),
		Stop:          day.Add(12 * time.Hour),
	}

	timer := domain.Timer{
		ID:        1,
		Active:    true,
		ChannelID: ch.ID,
		// Timer titles can be formatted differently than EPG titles (e.g. "alpha-retro|" vs "alpha-retro:").
		Title:     scheduled.Title + "|",
		Start:     scheduled.Start.Add(-2 * time.Minute),
		Stop:      scheduled.Stop.Add(2 * time.Minute),
	}

	mock := &searchVDRMock{
		channels: []domain.Channel{ch},
		epg:      []domain.EPGEvent{scheduled, other},
		timers:   []domain.Timer{timer},
	}

	epgService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "search_results.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		epgService,
		timerService,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"search_results.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/search?q=show", nil)
	req.Header.Set("HX-Request", "true")
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.EPGSearch(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	const recordActionPrefix = "hx-post=\"/timers/create?channel="
	const scheduledButton = "disabled>Scheduled</button>"

	scheduledIdx := strings.Index(body, scheduled.Title)
	otherIdx := strings.Index(body, other.Title)
	if scheduledIdx == -1 || otherIdx == -1 {
		t.Fatalf("expected both titles to render in HTML")
	}
	if !(scheduledIdx < otherIdx) {
		t.Fatalf("expected scheduled to appear before other")
	}

	scheduledSeg := body[scheduledIdx:otherIdx]
	otherSeg := body[otherIdx:]

	if !strings.Contains(scheduledSeg, scheduledButton) {
		t.Fatalf("expected scheduled show to be marked Scheduled")
	}
	if strings.Contains(scheduledSeg, recordActionPrefix) {
		t.Fatalf("did not expect record button for scheduled show")
	}
	if !strings.Contains(otherSeg, recordActionPrefix) {
		t.Fatalf("expected other show to remain recordable")
	}

	if got := strings.Count(body, scheduledButton); got != 1 {
		t.Fatalf("expected exactly 1 scheduled button, got %d", got)
	}
	if got := strings.Count(body, recordActionPrefix); got != 1 {
		t.Fatalf("expected exactly 1 record button, got %d", got)
	}
}
