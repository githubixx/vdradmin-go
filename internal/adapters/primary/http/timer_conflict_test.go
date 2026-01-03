package http

import (
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestCriticalTimerIDs_TransponderSharing_WithOneCard(t *testing.T) {
	day := time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local)

	t1 := domain.Timer{ID: 1, Active: true, ChannelID: "S19.2E-1-100-10", Start: day.Add(20 * time.Hour), Stop: day.Add(21 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: "S19.2E-1-100-11", Start: day.Add(20*time.Hour + 10*time.Minute), Stop: day.Add(20*time.Hour + 30*time.Minute)}

	crit := criticalTimerIDs([]domain.Timer{t1, t2}, 1, func(t domain.Timer) string {
		return transponderKeyFromChannelID(t.ChannelID)
	})
	if len(crit) != 0 {
		t.Fatalf("expected no critical timers (same transponder), got %v", crit)
	}
}

func TestCriticalTimerIDs_DifferentTransponders_ExceedsCards(t *testing.T) {
	day := time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local)

	t1 := domain.Timer{ID: 1, Active: true, ChannelID: "S19.2E-1-100-10", Start: day.Add(20 * time.Hour), Stop: day.Add(21 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: "S19.2E-1-200-20", Start: day.Add(20*time.Hour + 10*time.Minute), Stop: day.Add(20*time.Hour + 30*time.Minute)}

	crit := criticalTimerIDs([]domain.Timer{t1, t2}, 1, func(t domain.Timer) string {
		return transponderKeyFromChannelID(t.ChannelID)
	})
	if !crit[1] || !crit[2] {
		t.Fatalf("expected both timers critical with 1 card, got %v", crit)
	}
}

func TestTimerOverlapStates_RecurringTimer_IsMarkedToo(t *testing.T) {
	// Saturday 2026-01-03
	day := time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local)
	from := day
	to := day.Add(8 * 24 * time.Hour)

	// Recurring every Saturday: start 23:00, stop 01:00
	rec := domain.Timer{
		ID:           10,
		Active:       true,
		ChannelID:     "S19.2E-1-100-10",
		DaySpec:       "-----SS", // Sat+Sun allowed (position-based)
		StartMinutes:  23 * 60,
		StopMinutes:   1 * 60,
		Priority:     50,
		Lifetime:     99,
		Title:        "Tuff Stuff",
	}

	// Two one-time timers on Saturday overlapping the recurring slot.
	t1 := domain.Timer{ID: 1, Active: true, ChannelID: "S19.2E-1-200-20", Start: day.Add(20*time.Hour + 28*time.Minute), Stop: day.Add(23*time.Hour + 40*time.Minute)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: "S19.2E-1-300-30", Start: day.Add(22*time.Hour + 43*time.Minute), Stop: day.Add(23*time.Hour + 21*time.Minute)}

	collision, critical := timerOverlapStates([]domain.Timer{rec, t1, t2}, 1, from, to, func(t domain.Timer) string {
		return transponderKeyFromChannelID(t.ChannelID)
	})

	// With 1 DVB card and 2+ distinct transponders overlapping, all involved should be critical.
	if !critical[10] || !critical[1] || !critical[2] {
		t.Fatalf("expected all timers critical, got collision=%v critical=%v", collision, critical)
	}
}

func TestTimerOverlapStates_TwoCards_TwoTransponders_IsYellowNotRed(t *testing.T) {
	day := time.Date(2026, 1, 3, 0, 0, 0, 0, time.Local)
	from := day
	to := day.Add(24 * time.Hour)

	t1 := domain.Timer{ID: 1, Active: true, ChannelID: "S19.2E-1-100-10", Start: day.Add(20 * time.Hour), Stop: day.Add(21 * time.Hour)}
	t2 := domain.Timer{ID: 2, Active: true, ChannelID: "S19.2E-1-200-20", Start: day.Add(20*time.Hour + 10*time.Minute), Stop: day.Add(20*time.Hour + 30*time.Minute)}

	collision, critical := timerOverlapStates([]domain.Timer{t1, t2}, 2, from, to, func(t domain.Timer) string {
		return transponderKeyFromChannelID(t.ChannelID)
	})

	if critical[1] || critical[2] {
		t.Fatalf("expected no critical timers with 2 cards, got %v", critical)
	}
	if !collision[1] || !collision[2] {
		t.Fatalf("expected both timers collision (yellow) with overlap, got %v", collision)
	}
}
