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

type channelsVDRMock struct {
	channels []domain.Channel
	epqErr   error
	epq      []domain.EPGEvent
	timers   []domain.Timer
}

func (m *channelsVDRMock) Connect(ctx context.Context) error { return nil }
func (m *channelsVDRMock) Close() error                      { return nil }
func (m *channelsVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *channelsVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *channelsVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if m.epqErr != nil {
		return nil, m.epqErr
	}
	return m.epq, nil
}

func (m *channelsVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return m.timers, nil
}

func (m *channelsVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *channelsVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *channelsVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (m *channelsVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *channelsVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *channelsVDRMock) GetCurrentChannel(ctx context.Context) (string, error)  { return "", nil }
func (m *channelsVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *channelsVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func (m *channelsVDRMock) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}

func TestChannels_DisablesRecordOnlyForScheduledShow(t *testing.T) {
	loc := time.Local
	// Pick a day that is not "today" so the handler doesn't hide finished programs.
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(48 * time.Hour)
	dayStr := day.Format("2006-01-02")

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}

	scheduled := domain.EPGEvent{
		EventID:       100,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Landesschau Baden-Württemberg",
		Start:         day.Add(9*time.Hour + 10*time.Minute),
		Stop:          day.Add(10*time.Hour + 25*time.Minute),
	}
	before := domain.EPGEvent{
		EventID:       99,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Panoramablick",
		Start:         day.Add(7*time.Hour + 55*time.Minute),
		Stop:          day.Add(9*time.Hour + 10*time.Minute),
	}
	after := domain.EPGEvent{
		EventID:       101,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Eisenbahn-Romantik (503)",
		Start:         day.Add(10*time.Hour + 25*time.Minute),
		Stop:          day.Add(10*time.Hour + 55*time.Minute),
	}

	// Timer has margins that overlap neighboring shows.
	timer := domain.Timer{
		ID:        1,
		Active:    true,
		ChannelID: ch.ID,
		Title:     scheduled.Title,
		Start:     scheduled.Start.Add(-2 * time.Minute),
		Stop:      scheduled.Stop.Add(10 * time.Minute),
	}

	mock := &channelsVDRMock{
		channels: []domain.Channel{ch},
		epq:      []domain.EPGEvent{before, scheduled, after},
		timers:   []domain.Timer{timer},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "channels.html"),
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
	h.SetTemplates(map[string]*template.Template{"channels.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/channels?channel="+ch.ID+"&day="+dayStr, nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.Channels(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	const recordForm = "<form method=\"post\" action=\"/timers/create\""
	const scheduledButton = "disabled>Scheduled</button>"

	beforeIdx := strings.Index(body, before.Title)
	scheduledIdx := strings.Index(body, scheduled.Title)
	afterIdx := strings.Index(body, after.Title)

	if beforeIdx == -1 {
		t.Fatalf("expected to find before show title in HTML")
	}
	if scheduledIdx == -1 {
		t.Fatalf("expected to find scheduled show title in HTML")
	}
	if afterIdx == -1 {
		t.Fatalf("expected to find after show title in HTML")
	}
	if !(beforeIdx < scheduledIdx && scheduledIdx < afterIdx) {
		t.Fatalf("expected shows to render in EPG order (before < scheduled < after)")
	}

	beforeSeg := body[beforeIdx:scheduledIdx]
	scheduledSeg := body[scheduledIdx:afterIdx]
	afterSeg := body[afterIdx:]

	if !strings.Contains(beforeSeg, recordForm) {
		t.Fatalf("expected before show to remain recordable")
	}
	if !strings.Contains(afterSeg, recordForm) {
		t.Fatalf("expected after show to remain recordable")
	}
	if !strings.Contains(scheduledSeg, scheduledButton) {
		t.Fatalf("expected scheduled show to be marked Scheduled")
	}
	if strings.Contains(scheduledSeg, recordForm) {
		t.Fatalf("expected scheduled show not to render a record form")
	}

	if got := strings.Count(body, scheduledButton); got != 1 {
		t.Fatalf("expected exactly 1 scheduled button, got %d", got)
	}
	if got := strings.Count(body, recordForm); got != 2 {
		t.Fatalf("expected exactly 2 record forms (before/after), got %d", got)
	}
}

type channelsEPGAtSpyVDRMock struct {
	channels []domain.Channel

	gotChannelID string
	gotAt        time.Time
}

func (m *channelsEPGAtSpyVDRMock) Connect(ctx context.Context) error { return nil }
func (m *channelsEPGAtSpyVDRMock) Close() error                      { return nil }
func (m *channelsEPGAtSpyVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *channelsEPGAtSpyVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *channelsEPGAtSpyVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	m.gotChannelID = channelID
	m.gotAt = at
	return []domain.EPGEvent{}, nil
}

func (m *channelsEPGAtSpyVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return []domain.Timer{}, nil
}

func (m *channelsEPGAtSpyVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *channelsEPGAtSpyVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *channelsEPGAtSpyVDRMock) DeleteTimer(ctx context.Context, timerID int) error { return nil }

func (m *channelsEPGAtSpyVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}

func (m *channelsEPGAtSpyVDRMock) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}
func (m *channelsEPGAtSpyVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *channelsEPGAtSpyVDRMock) GetCurrentChannel(ctx context.Context) (string, error) {
	return "", nil
}
func (m *channelsEPGAtSpyVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *channelsEPGAtSpyVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestChannels_AnchorsEPGRequestToSelectedDay(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	selectedDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 2)
	dayStr := selectedDay.Format("2006-01-02")

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	spy := &channelsEPGAtSpyVDRMock{channels: []domain.Channel{ch}}

	epqService := services.NewEPGService(spy, 0)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "channels.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		epqService,
		nil,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"channels.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/channels?channel="+ch.ID+"&day="+dayStr, nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.Channels(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	if spy.gotChannelID != ch.ID {
		t.Fatalf("expected GetEPG to be called with channel %q, got %q", ch.ID, spy.gotChannelID)
	}
	if spy.gotAt.IsZero() {
		t.Fatalf("expected GetEPG to be called with a non-zero 'at' time")
	}
	if !spy.gotAt.Equal(selectedDay) {
		t.Fatalf("expected GetEPG 'at' to equal selected day start %s, got %s", selectedDay.Format(time.RFC3339), spy.gotAt.Format(time.RFC3339))
	}
}
