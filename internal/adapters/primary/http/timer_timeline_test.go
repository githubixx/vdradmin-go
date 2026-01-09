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
)

type timersTimelineVDRMock struct {
	channels []domain.Channel
	timers   []domain.Timer
}

func (m *timersTimelineVDRMock) Connect(ctx context.Context) error { return nil }
func (m *timersTimelineVDRMock) Close() error                      { return nil }
func (m *timersTimelineVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *timersTimelineVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return m.channels, nil
}

func (m *timersTimelineVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return nil, nil
}

func (m *timersTimelineVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return m.timers, nil
}

func (m *timersTimelineVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timersTimelineVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *timersTimelineVDRMock) DeleteTimer(ctx context.Context, timerID int) error { return nil }

func (m *timersTimelineVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}
func (m *timersTimelineVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *timersTimelineVDRMock) GetCurrentChannel(ctx context.Context) (string, error) {
	return "", nil
}
func (m *timersTimelineVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *timersTimelineVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestTimerList_RendersTimeline(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	ch1 := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}
	ch2 := domain.Channel{ID: "S19.2E-1-200-20", Number: 2, Name: "ZDF HD"}
	ch3 := domain.Channel{ID: "S19.2E-1-300-30", Number: 3, Name: "zdf_neo HD"}

	t1 := domain.Timer{ID: 1, Active: true, ChannelID: ch1.ID, Title: "Show A", Start: day.Add(1 * time.Hour), Stop: day.Add(2 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: ch2.ID, Title: "Show B", Start: day.Add(90 * time.Minute), Stop: day.Add(150 * time.Minute)}
	t3 := domain.Timer{ID: 3, Active: true, ChannelID: ch3.ID, Title: "Show C", Start: day.Add(5 * time.Hour), Stop: day.Add(6 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch1, ch2, ch3},
		timers:   []domain.Timer{t1, t2, t3},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	req := httptest.NewRequest(http.MethodGet, "/timers?day="+day.Format("2006-01-02"), nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerList(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	if !strings.Contains(body, "timer-timeline") {
		t.Fatalf("expected timeline container")
	}
	if !strings.Contains(body, "timeline-hours") {
		t.Fatalf("expected hours header")
	}
	if !strings.Contains(body, "SWR BW HD") || !strings.Contains(body, "ZDF HD") {
		t.Fatalf("expected channel names in timeline")
	}
	if !strings.Contains(body, "timeline-block collision") {
		t.Fatalf("expected collision (yellow) block")
	}
	if !strings.Contains(body, "timeline-block ok") {
		t.Fatalf("expected ok (green) block")
	}
}

func TestTimerList_TimelineDaysOnlyContainTimerDays_AndDoNotShiftWithSelection(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	day1 := todayStart.Add(24 * time.Hour)
	day2 := todayStart.Add(12 * 24 * time.Hour)

	ch1 := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}
	ch2 := domain.Channel{ID: "S19.2E-1-200-20", Number: 2, Name: "ZDF HD"}

	// Two one-time timers on different days.
	t1 := domain.Timer{ID: 1, Active: true, ChannelID: ch1.ID, Title: "Show A", Start: day1.Add(1 * time.Hour), Stop: day1.Add(2 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: ch2.ID, Title: "Show B", Start: day2.Add(3 * time.Hour), Stop: day2.Add(4 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch1, ch2},
		timers:   []domain.Timer{t1, t2},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	makeReq := func(rawURL string) string {
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
		ctx := context.WithValue(req.Context(), "user", "admin")
		ctx = context.WithValue(ctx, "role", "admin")
		req = req.WithContext(ctx)
		rw := httptest.NewRecorder()
		h.TimerList(rw, req)
		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		return rw.Body.String()
	}

	body1 := makeReq("/timers?day=" + day1.Format("2006-01-02"))
	if !strings.Contains(body1, "<option value=\""+day1.Format("2006-01-02")+"\"") {
		t.Fatalf("expected day1 to be present in dropdown")
	}
	if !strings.Contains(body1, "<option value=\""+day2.Format("2006-01-02")+"\"") {
		t.Fatalf("expected day2 to be present in dropdown")
	}

	body2 := makeReq("/timers?day=" + day2.Format("2006-01-02"))
	if !strings.Contains(body2, "<option value=\""+day1.Format("2006-01-02")+"\"") {
		t.Fatalf("expected day1 to be present in dropdown when selecting day2")
	}
	if !strings.Contains(body2, "<option value=\""+day2.Format("2006-01-02")+"\"") {
		t.Fatalf("expected day2 to be present in dropdown when selecting day2")
	}
}

func TestTimerList_SnapsSelectedDayToNearestAvailableTimerDay(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(2 * 24 * time.Hour)

	ch := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}
	t1 := domain.Timer{ID: 1, Active: true, ChannelID: ch.ID, Title: "Show A", Start: day.Add(1 * time.Hour), Stop: day.Add(2 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch},
		timers:   []domain.Timer{t1},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	// User selects a day with no timers; handler should select the nearest available day.
	selected := day.Add(24 * time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/timers?day="+selected.Format("2006-01-02"), nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)
	rw := httptest.NewRecorder()
	h.TimerList(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}

	body := rw.Body.String()
	if !strings.Contains(body, "<option value=\""+day.Format("2006-01-02")+"\" selected") {
		t.Fatalf("expected nearest available day to be selected")
	}
}

func TestTimerList_DoesNotIncludeMidnightEndDay(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	ch := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}
	// Timer that ends exactly at midnight of the next day.
	t1 := domain.Timer{ID: 1, Active: true, ChannelID: ch.ID, Title: "Late Show", Start: day.Add(23 * time.Hour), Stop: day.Add(24 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch},
		timers:   []domain.Timer{t1},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	// Request the next day; it should snap back to the day that actually has timer time.
	nextDay := day.Add(24 * time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/timers?day="+nextDay.Format("2006-01-02"), nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)
	rw := httptest.NewRecorder()
	h.TimerList(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := rw.Body.String()
	if strings.Contains(body, "<option value=\""+nextDay.Format("2006-01-02")+"\"") {
		t.Fatalf("did not expect midnight end day to be present in dropdown")
	}
	if !strings.Contains(body, "<option value=\""+day.Format("2006-01-02")+"\" selected") {
		t.Fatalf("expected actual timer day to be selected")
	}
}

func TestTimerList_RecurringTimerDaysCappedToNextWeek_OneTimeBeyondWeekStillIncluded(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Compute a Monday that will definitely be beyond the next-week horizon.
	daysUntilMon := (int(time.Monday) - int(todayStart.Weekday()) + 7) % 7
	firstMon := todayStart.Add(time.Duration(daysUntilMon) * 24 * time.Hour)
	thirdMon := firstMon.Add(14 * 24 * time.Hour)

	// Ensure farDay doesn't collide with the "third Monday" date.
	farDay := todayStart.Add(14 * 24 * time.Hour)
	if farDay.Equal(thirdMon) {
		farDay = farDay.Add(24 * time.Hour)
	}

	beyondWeekRecurringDay := thirdMon
	if beyondWeekRecurringDay.Equal(farDay) {
		beyondWeekRecurringDay = firstMon.Add(21 * 24 * time.Hour)
		if beyondWeekRecurringDay.Equal(farDay) {
			beyondWeekRecurringDay = firstMon.Add(28 * 24 * time.Hour)
		}
	}

	ch := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "SWR BW HD"}

	// Recurring weekly timer (Monday 08:00-09:00).
	weekly := domain.Timer{ID: 1, Active: true, ChannelID: ch.ID, Title: "Weekly", DaySpec: "M------", StartMinutes: 8 * 60, StopMinutes: 9 * 60}
	// One-time timer ~2 weeks out.
	oneTime := domain.Timer{ID: 2, Active: true, ChannelID: ch.ID, Title: "OneTime", Start: farDay.Add(20 * time.Hour), Stop: farDay.Add(21 * time.Hour)}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch},
		timers:   []domain.Timer{weekly, oneTime},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	req := httptest.NewRequest(http.MethodGet, "/timers?day="+todayStart.Format("2006-01-02"), nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerList(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := rw.Body.String()

	// The one-time timer day must be present even if it's beyond a week.
	if !strings.Contains(body, "<option value=\""+farDay.Format("2006-01-02")+"\"") {
		t.Fatalf("expected far-future one-time day to be present in dropdown")
	}

	// The recurring timer should contribute at least one day in the near horizon.
	if !strings.Contains(body, "<option value=\""+firstMon.Format("2006-01-02")+"\"") {
		t.Fatalf("expected first Monday occurrence day to be present in dropdown")
	}

	// But it must not extend the dropdown to occurrences beyond the next week horizon.
	if strings.Contains(body, "<option value=\""+beyondWeekRecurringDay.Format("2006-01-02")+"\"") {
		t.Fatalf("did not expect recurring occurrence beyond next-week horizon to be present in dropdown")
	}
}

func TestTimerList_RecurringThuFriMidnight_ShowsBothUpcomingOccurrencesInList(t *testing.T) {
	loc := time.Local
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Find the next Thursday and Friday (including today if it matches).
	daysUntilThu := (int(time.Thursday) - int(todayStart.Weekday()) + 7) % 7
	daysUntilFri := (int(time.Friday) - int(todayStart.Weekday()) + 7) % 7
	thu := todayStart.Add(time.Duration(daysUntilThu) * 24 * time.Hour)
	fri := todayStart.Add(time.Duration(daysUntilFri) * 24 * time.Hour)
	// Ensure Friday is after Thursday in the normal Thu/Fri pair.
	if !fri.After(thu) {
		fri = fri.Add(7 * 24 * time.Hour)
	}

	ch := domain.Channel{ID: "S19.2E-1-100-10", Number: 1, Name: "ROCK ANTENNE"}

	// Recurring weekly timer (Thu+Fri 00:00-01:00).
	weekly := domain.Timer{ID: 1, Active: true, ChannelID: ch.ID, Title: "Lange Nacht der ganzen Alben", DaySpec: "---TF--", StartMinutes: 0, StopMinutes: 60}

	mock := &timersTimelineVDRMock{
		channels: []domain.Channel{ch},
		timers:   []domain.Timer{weekly},
	}

	epqService := services.NewEPGService(mock, 0)
	timerService := services.NewTimerService(mock)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "timers.html"),
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
	h.SetTemplates(map[string]*template.Template{"timers.html": parsed})
	h.SetConfig(&config.Config{VDR: config.VDRConfig{DVBCards: 2}}, "")

	req := httptest.NewRequest(http.MethodGet, "/timers?day="+todayStart.Format("2006-01-02"), nil)
	ctx := context.WithValue(req.Context(), "user", "admin")
	ctx = context.WithValue(ctx, "role", "admin")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	h.TimerList(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rw.Code)
	}
	body := rw.Body.String()

	expThu := thu.Format("2006-01-02") + " 00:00 - 01:00"
	expFri := fri.Format("2006-01-02") + " 00:00 - 01:00"

	if !strings.Contains(body, expThu) {
		t.Fatalf("expected Thursday occurrence %q to be present", expThu)
	}
	if !strings.Contains(body, expFri) {
		t.Fatalf("expected Friday occurrence %q to be present", expFri)
	}
}
