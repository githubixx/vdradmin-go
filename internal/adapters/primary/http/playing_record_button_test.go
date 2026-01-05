package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"html/template"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

type playingVDRMock struct {
	channels []domain.Channel
	epqErr   error
	epq      []domain.EPGEvent
	timers   []domain.Timer
}

func (m *playingVDRMock) Connect(ctx context.Context) error { return nil }
func (m *playingVDRMock) Close() error                      { return nil }
func (m *playingVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *playingVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *playingVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if m.epqErr != nil {
		return nil, m.epqErr
	}
	return m.epq, nil
}

func (m *playingVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) { return m.timers, nil }

func (m *playingVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *playingVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *playingVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (m *playingVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *playingVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *playingVDRMock) GetCurrentChannel(ctx context.Context) (string, error)  { return "", nil }
func (m *playingVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *playingVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestPlayingToday_DisablesRecordWhenTimerExists(t *testing.T) {
	loc := time.Local
	day := time.Date(2026, 1, 4, 0, 0, 0, 0, loc)

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "Test Channel"}
	ev := domain.EPGEvent{
		EventID:       123,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Test Show",
		Start:         day.Add(10 * time.Hour),
		Stop:          day.Add(11 * time.Hour),
	}
	timer := domain.Timer{
		ID:        1,
		Active:    true,
		ChannelID: ch.ID,
		Title:     "Test Show",
		Start:     ev.Start.Add(-2 * time.Minute),
		Stop:      ev.Stop.Add(10 * time.Minute),
	}
	// Adjacent show that overlaps the timer window by margin only.
	adjacent := ev
	adjacent.EventID = 124
	adjacent.Title = "Other Show"
	adjacent.Start = day.Add(9*time.Hour + 55*time.Minute)
	adjacent.Stop = day.Add(10 * time.Hour)

	// Common real-world case: the EPG event has no ChannelNumber populated,
	// and timers reference channels by number string.
	evMissingNumber := ev
	evMissingNumber.EventID = 456
	evMissingNumber.ChannelNumber = 0
	evMissingNumber.ChannelName = ""
	timerNumericChannel := timer
	timerNumericChannel.ID = 2
	timerNumericChannel.ChannelID = "1"

	mock := &playingVDRMock{
		channels: []domain.Channel{ch},
		epq:      []domain.EPGEvent{ev, adjacent, evMissingNumber},
		timers:   []domain.Timer{timer, timerNumericChannel},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "playing.html"),
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
	h.SetTemplates(map[string]*template.Template{"playing.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/playing?day=2026-01-04", nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.PlayingToday(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	const recordForm = "<form method=\"post\" action=\"/timers/create\""
	const scheduledButton = "disabled>Scheduled</button>"

	// Two EPG items have the title "Test Show" (one with missing channel metadata).
	if got := strings.Count(body, scheduledButton); got != 2 {
		t.Fatalf("expected exactly 2 scheduled buttons (for both Test Show entries), got %d", got)
	}
	// Only the adjacent show should remain recordable.
	if got := strings.Count(body, recordForm); got != 1 {
		t.Fatalf("expected exactly 1 record form (for Other Show), got %d", got)
	}

	otherIdx := strings.Index(body, "Other Show")
	testIdx := strings.Index(body, "Test Show")
	if otherIdx == -1 || testIdx == -1 {
		t.Fatalf("expected both show titles to be present")
	}
	if otherIdx > testIdx {
		t.Fatalf("expected Other Show to appear before Test Show")
	}
	otherSeg := body[otherIdx:testIdx]
	if !strings.Contains(otherSeg, recordForm) {
		t.Fatalf("expected Other Show to still be recordable")
	}

	scheduledSeg := body[testIdx:]
	if !strings.Contains(scheduledSeg, scheduledButton) {
		t.Fatalf("expected Test Show to be marked Scheduled")
	}
	if strings.Contains(scheduledSeg, recordForm) {
		t.Fatalf("did not expect record form for scheduled items")
	}
}
