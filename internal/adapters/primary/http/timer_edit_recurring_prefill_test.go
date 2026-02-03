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

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

func TestTimerEdit_PrefillsWeeklyTimer(t *testing.T) {
	ch := domain.Channel{ID: "C-1-2-3", Number: 1, Name: "SWR BW HD"}
	weekly := domain.Timer{
		ID:           7,
		Active:       true,
		ChannelID:    ch.ID,
		DaySpec:      "M-W----",
		StartMinutes: 8 * 60,
		StopMinutes:  9*60 + 30,
		Priority:     50,
		Lifetime:     99,
		Title:        "Recurring",
		Aux:          "",
	}

	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{ch}).
		WithTimers([]domain.Timer{weekly})

	epgService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timer_edit.html"),
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
	h.SetTemplates(map[string]*template.Template{"timer_edit.html": parsed})

	req := httptest.NewRequest(http.MethodGet, "/timers/edit?id=7", nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerEdit(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := rw.Body.String()

	if !strings.Contains(body, "name=\"day_mode\" value=\"weekly\" checked") {
		t.Fatalf("expected weekly day mode to be selected")
	}
	if !strings.Contains(body, "name=\"wd_mon\" value=\"1\" checked") {
		t.Fatalf("expected Monday to be checked")
	}
	if !strings.Contains(body, "name=\"wd_wed\" value=\"1\" checked") {
		t.Fatalf("expected Wednesday to be checked")
	}
	if !strings.Contains(body, "name=\"start\" value=\"08:00\"") {
		t.Fatalf("expected start to be formatted from minutes")
	}
	if !strings.Contains(body, "name=\"stop\" value=\"09:30\"") {
		t.Fatalf("expected stop to be formatted from minutes")
	}
	if !strings.Contains(body, "<option value=\"C-1-2-3\" selected>") {
		t.Fatalf("expected channel to be selected")
	}
}
