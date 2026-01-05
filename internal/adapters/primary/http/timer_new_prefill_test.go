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

type timerNewPrefillVDRMock struct {
	channels []domain.Channel
}

func (m *timerNewPrefillVDRMock) Connect(ctx context.Context) error { return nil }
func (m *timerNewPrefillVDRMock) Close() error                      { return nil }
func (m *timerNewPrefillVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *timerNewPrefillVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *timerNewPrefillVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return []domain.EPGEvent{}, nil
}

func (m *timerNewPrefillVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return []domain.Timer{}, nil
}

func (m *timerNewPrefillVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timerNewPrefillVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timerNewPrefillVDRMock) DeleteTimer(ctx context.Context, timerID int) error { return nil }

func (m *timerNewPrefillVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *timerNewPrefillVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *timerNewPrefillVDRMock) GetCurrentChannel(ctx context.Context) (string, error) {
	return "", nil
}
func (m *timerNewPrefillVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *timerNewPrefillVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestTimerNew_PrefillsFromQueryParams(t *testing.T) {
	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	mock := &timerNewPrefillVDRMock{channels: []domain.Channel{ch}}

	epqService := services.NewEPGService(mock, 0)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timer_edit.html"),
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
	h.SetTemplates(map[string]*template.Template{"timer_edit.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/timers/new?channel=C-1-2-3&day=2026-01-07&start=00:00&stop=01:30&title=Familie+Heinz+Becker+-+Lachgeschichten", nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerNew(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "value=\"2026-01-07\"") {
		t.Fatalf("expected day to be prefilled")
	}
	if !strings.Contains(body, "name=\"start\" value=\"00:00\"") {
		t.Fatalf("expected start to be prefilled")
	}
	if !strings.Contains(body, "name=\"stop\" value=\"01:30\"") {
		t.Fatalf("expected stop to be prefilled")
	}
	if !strings.Contains(body, "name=\"title\" value=\"Familie Heinz Becker - Lachgeschichten\"") {
		t.Fatalf("expected title to be prefilled")
	}
	if !strings.Contains(body, "<option value=\"C-1-2-3\" selected>") {
		t.Fatalf("expected channel to be selected")
	}
}
