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

type whatsOnVDRMock struct {
	epg      []domain.EPGEvent
	channels []domain.Channel
}

func (m *whatsOnVDRMock) Connect(ctx context.Context) error { return nil }
func (m *whatsOnVDRMock) Close() error                      { return nil }
func (m *whatsOnVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *whatsOnVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}
func (m *whatsOnVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return m.epg, nil
}

func (m *whatsOnVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return []domain.Timer{}, nil
}
func (m *whatsOnVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *whatsOnVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (m *whatsOnVDRMock) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (m *whatsOnVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *whatsOnVDRMock) DeleteRecording(ctx context.Context, path string) error        { return nil }
func (m *whatsOnVDRMock) GetCurrentChannel(ctx context.Context) (string, error)         { return "", nil }
func (m *whatsOnVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error { return nil }
func (m *whatsOnVDRMock) SendKey(ctx context.Context, key string) error                 { return nil }

func TestWhatsOnNow_AtTimeSelectsPrograms(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	ch1 := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	ch2 := domain.Channel{ID: "C-2-3-4", Number: 2, Name: "ZDF HD"}

	m := &whatsOnVDRMock{channels: []domain.Channel{ch1, ch2}}
	m.epg = []domain.EPGEvent{
		{ChannelID: ch1.ID, ChannelNumber: ch1.Number, ChannelName: ch1.Name, Title: "Show A", Start: day.Add(18 * time.Hour), Stop: day.Add(19 * time.Hour)},
		{ChannelID: ch1.ID, ChannelNumber: ch1.Number, ChannelName: ch1.Name, Title: "Show B", Start: day.Add(19 * time.Hour), Stop: day.Add(20 * time.Hour)},
		{ChannelID: ch2.ID, ChannelNumber: ch2.Number, ChannelName: ch2.Name, Title: "News", Start: day.Add(18 * time.Hour), Stop: day.Add(19 * time.Hour)},
	}

	epgService := services.NewEPGService(m, 0)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "index.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		epgService,
		nil,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"index.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/now?h=18&at=18:30", nil)
	rw := httptest.NewRecorder()
	h.WhatsOnNow(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	if !strings.Contains(body, "Show A") {
		t.Fatalf("expected Show A at 18:30")
	}
	if !strings.Contains(body, "News") {
		t.Fatalf("expected News at 18:30")
	}
	if strings.Contains(body, "Show B") {
		t.Fatalf("did not expect Show B at 18:30")
	}
}

func TestWhatsOnNow_InvalidTimeShowsErrorAndFallsBack(t *testing.T) {
	m := &whatsOnVDRMock{channels: []domain.Channel{}}
	epgService := services.NewEPGService(m, 0)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "index.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		epgService,
		nil,
		nil,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{"index.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/now?at=25:00", nil)
	rw := httptest.NewRecorder()
	h.WhatsOnNow(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := strings.ToLower(rw.Body.String())
	if !strings.Contains(body, "invalid time") {
		t.Fatalf("expected invalid time error")
	}
}
