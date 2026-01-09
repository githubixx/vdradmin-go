package http

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestHandler_timerFromForm_OvernightStopRollsToNextDay(t *testing.T) {
	h := &Handler{}

	form := url.Values{}
	form.Set("id", "7")
	form.Set("active", "1")
	form.Set("channel", "C-1-2-3")
	form.Set("day", "2026-01-03")
	form.Set("start", "23:30")
	form.Set("stop", "00:30")
	form.Set("priority", "50")
	form.Set("lifetime", "99")
	form.Set("title", "Overnight")
	form.Set("aux", "")

	req := &http.Request{Form: form}

	timer, err := h.timerFromForm(req)
	if err != nil {
		t.Fatalf("timerFromForm: %v", err)
	}

	if timer.ID != 7 {
		t.Fatalf("expected ID=7, got %d", timer.ID)
	}
	if timer.ChannelID != "C-1-2-3" {
		t.Fatalf("expected channel, got %q", timer.ChannelID)
	}
	if timer.Title != "Overnight" {
		t.Fatalf("expected title, got %q", timer.Title)
	}

	if timer.Start.Format("2006-01-02 15:04") != "2026-01-03 23:30" {
		t.Fatalf("unexpected start: %s", timer.Start.Format(time.RFC3339))
	}
	if timer.Stop.Format("2006-01-02 15:04") != "2026-01-04 00:30" {
		t.Fatalf("unexpected stop: %s", timer.Stop.Format(time.RFC3339))
	}

	if timer.Stop.Before(timer.Start) {
		t.Fatalf("stop should be after start")
	}
}

func TestHandler_timerFromForm_InvalidInput(t *testing.T) {
	h := &Handler{}
	form := url.Values{}
	form.Set("id", "0")
	req := &http.Request{Form: form}

	_, err := h.timerFromForm(req)
	if err != domain.ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestHandler_timerFromForm_WeeklyMask(t *testing.T) {
	h := &Handler{}

	form := url.Values{}
	form.Set("id", "7")
	form.Set("active", "1")
	form.Set("channel", "C-1-2-3")
	form.Set("day_mode", "weekly")
	form.Set("wd_sat", "1")
	form.Set("start", "23:00")
	form.Set("stop", "01:00")
	form.Set("priority", "50")
	form.Set("lifetime", "99")
	form.Set("title", "Recurring")
	form.Set("aux", "")

	req := &http.Request{Form: form}

	timer, err := h.timerFromForm(req)
	if err != nil {
		t.Fatalf("timerFromForm: %v", err)
	}

	if timer.DaySpec != "-----S-" {
		t.Fatalf("expected DaySpec=-----S-, got %q", timer.DaySpec)
	}
	if timer.StartMinutes != 23*60 {
		t.Fatalf("expected StartMinutes=1380, got %d", timer.StartMinutes)
	}
	if timer.StopMinutes != 1*60 {
		t.Fatalf("expected StopMinutes=60, got %d", timer.StopMinutes)
	}
}
