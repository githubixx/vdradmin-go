package http

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

type epgsearchRunVDRMock struct {
	channels []domain.Channel
	epq      []domain.EPGEvent
	timers   []domain.Timer
}

func (m *epgsearchRunVDRMock) Connect(ctx context.Context) error { return nil }
func (m *epgsearchRunVDRMock) Close() error                      { return nil }
func (m *epgsearchRunVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *epgsearchRunVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *epgsearchRunVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	// For this test we always return the full set; filtering is done by the handler/services.
	return m.epq, nil
}

func (m *epgsearchRunVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return m.timers, nil
}

func (m *epgsearchRunVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *epgsearchRunVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *epgsearchRunVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (m *epgsearchRunVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}

func (m *epgsearchRunVDRMock) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}
func (m *epgsearchRunVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *epgsearchRunVDRMock) GetCurrentChannel(ctx context.Context) (string, error)  { return "", nil }
func (m *epgsearchRunVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *epgsearchRunVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestEPGSearchNew_RunRendersMatchesAndRecordPrefillLink(t *testing.T) {
	loc := time.Local
	eventStart := time.Date(2026, 1, 7, 0, 0, 0, 0, loc)
	eventStop := time.Date(2026, 1, 7, 1, 30, 0, 0, loc)

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	ev := domain.EPGEvent{
		EventID:       123,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Familie Heinz Becker - Lachgeschichten",
		Start:         eventStart,
		Stop:          eventStop,
	}

	mock := &epgsearchRunVDRMock{
		channels: []domain.Channel{ch},
		epq:      []domain.EPGEvent{ev},
	}
	EPG := services.NewEPGService(mock, 0)
	Timers := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_edit.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		EPG,
		Timers,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"epgsearch_edit.html": parsed})

	form := url.Values{}
	form.Set("action", "run")
	form.Set("active", "on")
	form.Set("pattern", "Heinz")
	form.Set("mode", "phrase")
	form.Set("in_title", "on")
	form.Set("in_subtitle", "on")
	form.Set("in_description", "on")
	form.Set("use_channel", "no")

	req := httptest.NewRequest(http.MethodPost, "/epgsearch/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.EPGSearchCreate(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	mustContain(t, body, "Add New Search")
	mustContain(t, body, "Matches")
	mustContain(t, body, ev.Title)
	mustContain(t, body, "value=\"Heinz\"")

	mustContain(t, body, "/timers/new?channel="+url.QueryEscape(ch.ID))
	mustContain(t, body, "title=")
	mustContain(t, body, "day=2026-01-07")
	mustContain(t, body, "start=00")
	mustContain(t, body, "stop=01")
}

func TestEPGSearchNew_RunMarksMidnightMatchAsScheduledWithNumericChannelTimer(t *testing.T) {
	loc := time.Local
	eventStart := time.Date(2026, 1, 7, 0, 0, 0, 0, loc)
	eventStop := time.Date(2026, 1, 7, 1, 30, 0, 0, loc)

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	ev := domain.EPGEvent{
		EventID:       123,
		ChannelID:     ch.ID,
		ChannelNumber: 0, // simulate missing numeric channel metadata in EPG output
		ChannelName:   ch.Name,
		Title:         "Familie Heinz Becker - Lachgeschichten",
		Start:         eventStart,
		Stop:          eventStop,
	}

	// Timer created with margins that start on the previous day, and stored using the numeric
	// channel reference (as VDR often does). Should still mark the midnight event as scheduled.
	timer := domain.Timer{
		ID:        42,
		Active:    true,
		ChannelID: "1",
		Title:     ev.Title,
		Start:     time.Date(2026, 1, 6, 23, 58, 0, 0, loc),
		Stop:      time.Date(2026, 1, 7, 1, 40, 0, 0, loc),
	}

	mock := &epgsearchRunVDRMock{
		channels: []domain.Channel{ch},
		epq:      []domain.EPGEvent{ev},
		timers:   []domain.Timer{timer},
	}
	EPG := services.NewEPGService(mock, 0)
	Timers := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_edit.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		EPG,
		Timers,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"epgsearch_edit.html": parsed})

	form := url.Values{}
	form.Set("action", "run")
	form.Set("active", "on")
	form.Set("pattern", "Heinz")
	form.Set("mode", "phrase")
	form.Set("in_title", "on")
	form.Set("in_subtitle", "on")
	form.Set("in_description", "on")
	form.Set("use_channel", "no")

	req := httptest.NewRequest(http.MethodPost, "/epgsearch/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.EPGSearchCreate(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	mustContain(t, body, ev.Title)
	mustContain(t, body, "Scheduled")
	if strings.Contains(body, "/timers/new?channel=") {
		t.Fatalf("expected no Record link when scheduled; got %q", body)
	}
}

func TestEPGSearchEdit_RunRendersMatches(t *testing.T) {
	loc := time.Local
	eventStart := time.Date(2026, 1, 7, 0, 0, 0, 0, loc)
	eventStop := time.Date(2026, 1, 7, 1, 30, 0, 0, loc)

	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	ev := domain.EPGEvent{
		EventID:       123,
		ChannelID:     ch.ID,
		ChannelNumber: ch.Number,
		ChannelName:   ch.Name,
		Title:         "Familie Heinz Becker - Lachgeschichten",
		Start:         eventStart,
		Stop:          eventStop,
	}

	mock := &epgsearchRunVDRMock{
		channels: []domain.Channel{ch},
		epq:      []domain.EPGEvent{ev},
	}
	EPG := services.NewEPGService(mock, 0)
	Timers := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_edit.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		EPG,
		Timers,
		nil,
		nil,
	)
	// EPGSearchUpdate requires a cfg for save, but action=run should work without it.
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"epgsearch_edit.html": parsed})

	form := url.Values{}
	form.Set("action", "run")
	form.Set("id", "1")
	form.Set("active", "on")
	form.Set("pattern", "Heinz")
	form.Set("mode", "phrase")
	form.Set("in_title", "on")
	form.Set("in_subtitle", "on")
	form.Set("in_description", "on")
	form.Set("use_channel", "no")

	req := httptest.NewRequest(http.MethodPost, "/epgsearch/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.EPGSearchUpdate(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	mustContain(t, body, "Edit Search")
	mustContain(t, body, "Matches")
	mustContain(t, body, ev.Title)
	mustContain(t, body, "value=\"Heinz\"")
}
