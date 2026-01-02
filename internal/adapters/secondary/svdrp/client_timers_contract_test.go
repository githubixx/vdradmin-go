package svdrp_test

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/adapters/secondary/svdrp"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestClient_GetTimers_ParsesTimerFields(t *testing.T) {
	// Use local times because the SVDRP client formats/parses timers in time.Local.
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "LSTT", respond: []string{
				"250-1 1:C-1-2-3:2026-01-02:2015:2115:50:99:My Title:Aux",
				"250 1 timers",
			}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	timers, err := c.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers: %v", err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}

	tm := timers[0]
	if tm.ID != 1 {
		t.Fatalf("expected ID=1, got %d", tm.ID)
	}
	if !tm.Active {
		t.Fatalf("expected Active=true")
	}
	if tm.ChannelID != "C-1-2-3" {
		t.Fatalf("expected ChannelID %q, got %q", "C-1-2-3", tm.ChannelID)
	}
	if tm.Priority != 50 {
		t.Fatalf("expected Priority=50, got %d", tm.Priority)
	}
	if tm.Lifetime != 99 {
		t.Fatalf("expected Lifetime=99, got %d", tm.Lifetime)
	}
	if tm.Title != "My Title" {
		t.Fatalf("expected Title %q, got %q", "My Title", tm.Title)
	}
	if tm.Aux != "Aux" {
		t.Fatalf("expected Aux %q, got %q", "Aux", tm.Aux)
	}

	// Day and derived timestamps are parsed in local time.
	if tm.Day.Year() != day.Year() || tm.Day.Month() != day.Month() || tm.Day.Day() != day.Day() {
		t.Fatalf("expected Day %s, got %s", day.Format("2006-01-02"), tm.Day.Format("2006-01-02"))
	}
	if tm.Start.Hour() != 20 || tm.Start.Minute() != 15 {
		t.Fatalf("expected Start 20:15, got %s", tm.Start.Format("15:04"))
	}
	if tm.Stop.Hour() != 21 || tm.Stop.Minute() != 15 {
		t.Fatalf("expected Stop 21:15, got %s", tm.Stop.Format("15:04"))
	}
}

func TestClient_TimerWrites_SendExpectedCommands(t *testing.T) {
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	start := time.Date(2026, 1, 2, 20, 15, 0, 0, time.Local)
	stop := time.Date(2026, 1, 2, 21, 15, 0, 0, time.Local)

	// Colons should be sanitized into pipes.
	timer := &domain.Timer{
		Active:    true,
		ChannelID: "C-1-2-3",
		Day:       day,
		Start:     start,
		Stop:      stop,
		Priority:  50,
		Lifetime:  99,
		Title:     "My:Title",
		Aux:       "Aux:Field",
	}
	formatted := "1:C-1-2-3:2026-01-02:2015:2115:50:99:My|Title:Aux|Field"

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "NEWT " + formatted, respond: []string{"250 ok"}},
			{expect: "MODT 7 " + formatted, respond: []string{"250 ok"}},
			{expect: "DELT 7", respond: []string{"250 ok"}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.CreateTimer(ctx, timer); err != nil {
		t.Fatalf("CreateTimer: %v", err)
	}

	timer.ID = 7
	if err := c.UpdateTimer(ctx, timer); err != nil {
		t.Fatalf("UpdateTimer: %v", err)
	}

	if err := c.DeleteTimer(ctx, 7); err != nil {
		t.Fatalf("DeleteTimer: %v", err)
	}
}

func TestClient_GetTimers_OvernightTimer_StopRollsToNextDay(t *testing.T) {
	// Start at 23:30, stop at 00:30 (next day).
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)

	srv := newSVDRPTestServer(t, []svdrpConnScript{{
		steps: []svdrpConnStep{
			{expect: "LSTT", respond: []string{
				"250-1 1:C-1-2-3:2026-01-02:2330:0030:50:99:Overnight:Aux",
				"250 1 timers",
			}},
		},
	}})
	defer srv.Close()

	host, port := srv.Addr()
	c := svdrp.NewClient(host, port, 2*time.Second)
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	timers, err := c.GetTimers(ctx)
	if err != nil {
		t.Fatalf("GetTimers: %v", err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}

	tm := timers[0]
	if tm.Day.Year() != day.Year() || tm.Day.Month() != day.Month() || tm.Day.Day() != day.Day() {
		t.Fatalf("expected Day %s, got %s", day.Format("2006-01-02"), tm.Day.Format("2006-01-02"))
	}
	if tm.Start.Format("2006-01-02 15:04") != "2026-01-02 23:30" {
		t.Fatalf("expected Start %q, got %q", "2026-01-02 23:30", tm.Start.Format("2006-01-02 15:04"))
	}
	if tm.Stop.Format("2006-01-02 15:04") != "2026-01-03 00:30" {
		t.Fatalf("expected Stop %q, got %q", "2026-01-03 00:30", tm.Stop.Format("2006-01-02 15:04"))
	}
}
