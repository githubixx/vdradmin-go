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

func TestSearch_SortsDayGroupsChronologically(t *testing.T) {
	loc := time.Local

	// Intentionally provide events out of order (later date first).
	later := domain.EPGEvent{
		EventID:       200,
		ChannelID:     "C-1-2-3",
		ChannelNumber: 1,
		ChannelName:   "3sat HD",
		Title:         "Schmidbauer & Kälberer: Ringlstetter - LIVE 2024",
		Start:         time.Date(2026, 1, 27, 3, 0, 0, 0, loc),
		Stop:          time.Date(2026, 1, 27, 3, 45, 0, 0, loc),
	}
	sooner := domain.EPGEvent{
		EventID:       201,
		ChannelID:     "C-9-9-9",
		ChannelNumber: 9,
		ChannelName:   "BR Fernsehen Nord HD",
		Title:         "Ringlstetter Retro (2/7)",
		Start:         time.Date(2026, 1, 22, 22, 0, 0, 0, loc),
		Stop:          time.Date(2026, 1, 22, 22, 45, 0, 0, loc),
	}

	mock := &searchVDRMock{
		channels: []domain.Channel{
			{ID: "C-1-2-3", Number: 1, Name: "3sat HD"},
			{ID: "C-9-9-9", Number: 9, Name: "BR Fernsehen Nord HD"},
		},
		epg:    []domain.EPGEvent{later, sooner},
		timers: nil,
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
	h.SetTemplates(map[string]*template.Template{"search_results.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/search?q=ringlstetter", nil)
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
	// Day labels are formatted as "Mon 2006-01-02".
	idxSooner := strings.Index(body, "Thu 2026-01-22")
	idxLater := strings.Index(body, "Tue 2026-01-27")
	if idxSooner == -1 || idxLater == -1 {
		t.Fatalf("expected both day headers to render")
	}
	if idxSooner > idxLater {
		t.Fatalf("expected earlier day group to render first")
	}
}
