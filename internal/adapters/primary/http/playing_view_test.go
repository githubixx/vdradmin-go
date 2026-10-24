package http

import (
	"context"
	"errors"
	"html"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestPlayingToday_Views(t *testing.T) {
	day := time.Date(2026, 1, 4, 0, 0, 0, 0, time.Local)
	ch := domain.Channel{ID: "C-1", Number: 1, Name: "Test Channel"}
	event := domain.EPGEvent{
		EventID: 1, ChannelID: ch.ID, ChannelNumber: 1, Title: "First show",
		Start: day.Add(10 * time.Hour), Stop: day.Add(11 * time.Hour),
	}
	later := event
	later.EventID, later.Title = 2, "Second show"
	later.Start, later.Stop = day.Add(11*time.Hour), day.Add(12*time.Hour)
	parsed := template.Must(ParsePageTemplates(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "playing.html"),
	))

	for _, tc := range []struct {
		name, query, role string
		empty, failure    bool
		status            int
	}{
		{name: "default", role: "admin", status: 200},
		{name: "list", query: "&view=list", role: "admin", status: 200},
		{name: "guide", query: "&view=guide", role: "admin", status: 200},
		{name: "midnight list", query: "&start=23:00&end=24:00", empty: true, status: 200},
		{name: "midnight guide", query: "&view=guide&start=23:00&end=24:00", empty: true, status: 200},
		{name: "viewer guide", query: "&view=guide", role: "viewer", status: 200},
		{name: "filtered", query: "&view=guide&start=11:00", role: "admin", status: 200},
		{name: "empty", query: "&view=guide", empty: true, status: 200},
		{name: "backend failure", query: "&view=guide", failure: true, status: 200},
		{name: "invalid", query: "&view=bogus", status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &playingVDRMock{
				channels: []domain.Channel{ch}, epq: []domain.EPGEvent{later, event},
				timers: []domain.Timer{{
					ID: 1, ChannelID: ch.ID, Title: event.Title,
					Start: event.Start, Stop: event.Stop,
				}},
			}
			if tc.empty {
				mock.epq = nil
			}
			if tc.failure {
				mock.epqErr = errors.New("fixture EPG unavailable")
			}
			h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), parsed,
				services.NewEPGService(mock, 0), services.NewTimerService(mock), nil, nil)
			h.SetTemplates(map[string]*template.Template{"playing.html": parsed})
			req := httptest.NewRequest("GET", "/playing?day=2026-01-04&channel-jump=ch-1"+tc.query, nil)
			req = req.WithContext(context.WithValue(req.Context(), "role", tc.role))
			res := httptest.NewRecorder()
			h.PlayingToday(res, req)
			if res.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if tc.status != http.StatusOK {
				return
			}
			body := res.Body.String()
			guide := strings.Contains(tc.query, "view=guide")
			if strings.Contains(body, `container page-playing playing-guide`) != guide {
				t.Fatal("incorrect layout class")
			}
			if !strings.Contains(body, `class="playing-pager ui-pager toolbar"`) {
				t.Fatal("missing shared Chroma UI pager composition")
			}
			for _, id := range []string{"playing-start", "playing-end"} {
				control := regexp.MustCompile(`(?s)<select id="` + id + `".*?</select>`).FindString(body)
				if control == "" {
					t.Fatalf("missing hourly select %s", id)
				}
				wantCount := 24
				if id == "playing-end" {
					wantCount = 25
					if !strings.Contains(control, `value="00:00" disabled`) {
						t.Fatal("end before the earliest start is not disabled")
					}
					if strings.Contains(tc.query, "start=23:00") &&
						(!strings.Contains(control, `value="23:00" disabled`) ||
							!strings.Contains(control, `value="24:00" selected`)) {
						t.Fatal("23:00 start must select midnight and disable earlier end options")
					}
				}
				if got := strings.Count(control, "<option "); got != wantCount {
					t.Fatalf("%s options = %d, want %d", id, got, wantCount)
				}
			}
			if tc.failure {
				if !strings.Contains(body, "fixture EPG unavailable") {
					t.Fatal("missing explicit connection error")
				}
				return
			}
			if tc.empty {
				if !strings.Contains(body, "No programs found") {
					t.Fatal("missing empty state")
				}
				if !strings.Contains(body, `value="ch-1" selected>Selected channel unavailable`) {
					t.Fatal("empty schedule lost the selected channel")
				}
				return
			}
			filtered := strings.Contains(tc.query, "start=11:00")
			if !filtered && strings.Index(body, "First show") > strings.Index(body, "Second show") {
				t.Fatal("programmes are not chronological")
			}
			if filtered && strings.Contains(body, "First show") {
				t.Fatal("finished programme was not filtered")
			}
			wantRecords := 0
			if tc.role == "admin" {
				wantRecords = 1
			}
			if got := strings.Count(body, `action="/timers/create"`); got != wantRecords {
				t.Fatalf("record forms = %d, want %d", got, wantRecords)
			}
			if tc.role == "admin" && !filtered && !strings.Contains(body, "disabled>Scheduled</button>") {
				t.Fatal("missing scheduled timer state")
			}
			links := regexp.MustCompile(`data-playing-day href="([^"]+)"`).FindAllStringSubmatch(body, -1)
			if len(links) != 2 {
				t.Fatal("expected both day navigation links")
			}
			for i, link := range links {
				u, err := url.Parse(html.UnescapeString(link[1]))
				if err != nil {
					t.Fatal(err)
				}
				query := u.Query()
				wantDay := []string{"2026-01-03", "2026-01-05"}[i]
				wantView, wantStart := "list", "00:00"
				if guide {
					wantView = "guide"
				}
				if filtered {
					wantStart = "11:00"
				}
				if query.Get("day") != wantDay || query.Get("view") != wantView ||
					query.Get("start") != wantStart || query.Get("end") != "24:00" ||
					query.Get("channel-jump") != "ch-1" {
					t.Fatalf("navigation lost state: %s", u)
				}
			}
		})
	}
}
