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
	"github.com/githubixx/vdradmin-go/internal/ports"
)

func TestTimerCreate_UsesConfiguredDefaultMargins(t *testing.T) {
	start := time.Date(2026, 1, 3, 20, 0, 0, 0, time.Local)
	stop := time.Date(2026, 1, 3, 21, 0, 0, 0, time.Local)

	var mu sync.Mutex
	var created []*domain.Timer

	mock := ports.NewMockVDRClient().WithEPGEvents([]domain.EPGEvent{{
		EventID:   123,
		ChannelID: "C-1-2-3",
		Title:     "Show",
		Start:     start,
		Stop:      stop,
	}})
	mock.CreateTimerFunc = func(ctx context.Context, timer *domain.Timer) error {
		mu.Lock()
		defer mu.Unlock()
		cp := *timer
		created = append(created, &cp)
		return nil
	}

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

	mu.Lock()
	defer mu.Unlock()
	if len(created) != 1 {
		t.Fatalf("expected 1 created timer, got %d", len(created))
	}
	timer := created[0]

	expectedStart := start.Add(-7 * time.Minute)
	expectedStop := stop.Add(11 * time.Minute)
	if !timer.Start.Equal(expectedStart) {
		t.Fatalf("expected Start %s, got %s", expectedStart.Format(time.RFC3339), timer.Start.Format(time.RFC3339))
	}
	if !timer.Stop.Equal(expectedStop) {
		t.Fatalf("expected Stop %s, got %s", expectedStop.Format(time.RFC3339), timer.Stop.Format(time.RFC3339))
	}
}
