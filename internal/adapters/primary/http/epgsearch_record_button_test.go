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
	"github.com/githubixx/vdradmin-go/internal/ports"
)

func TestEPGSearch_DisablesRecordWhenTimerOverlaps(t *testing.T) {
	loc := time.Local
	day := time.Date(2026, 1, 10, 0, 0, 0, 0, loc)

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
		Start:         day.Add(10 * time.Hour),
		Stop:          day.Add(11 * time.Hour),
	}

	// Timer overlaps "Show A" but has a different title (common in practice).
	// We still must prevent duplicate recordings.
	timer := domain.Timer{
		ID:        1,
		Active:    true,
		ChannelID: ch.ID,
		Title:     "Some existing timer",
		Start:     scheduled.Start.Add(-2 * time.Minute),
		Stop:      scheduled.Stop.Add(2 * time.Minute), // overlaps Show B by 2 minutes
	}

	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{ch}).
		WithEPGEvents([]domain.EPGEvent{scheduled, other}).
		WithTimers([]domain.Timer{timer})

	epgService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_results.html"),
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
	h.SetTemplates(map[string]*template.Template{"epgsearch_results.html": parsed})
	h.SetConfig(&config.Config{EPG: config.EPGConfig{Searches: []config.EPGSearch{{
		ID:         1,
		Active:     true,
		Mode:       "phrase",
		Pattern:    "Show",
		InTitle:    true,
		UseChannel: "no",
	}}}}, "")

	form := strings.NewReader("ids=1")
	req := httptest.NewRequest(http.MethodPost, "/epgsearch/execute", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.EPGSearchExecute(rw, req)

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
}
