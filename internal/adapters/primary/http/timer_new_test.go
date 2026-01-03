package http

import (
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestHandler_timerNewFormModel_Defaults(t *testing.T) {
	h := &Handler{}

	now := time.Date(2026, 1, 3, 15, 4, 59, 0, time.Local)
	channels := []domain.Channel{{ID: "C-1-2-3", Name: "One"}, {ID: "C-4-5-6", Name: "Two"}}

	model, selected := h.timerNewFormModel(now, channels)

	if !model.Active {
		t.Fatalf("expected Active=true")
	}
	if selected != "C-1-2-3" {
		t.Fatalf("expected selected channel to be first, got %q", selected)
	}
	if model.Day != "2026-01-03" {
		t.Fatalf("expected Day=2026-01-03, got %q", model.Day)
	}
	if model.Start != "15:04" {
		t.Fatalf("expected Start=15:04, got %q", model.Start)
	}
	if model.Stop != "00:00" {
		t.Fatalf("expected Stop=00:00, got %q", model.Stop)
	}
	if model.Priority != 99 || model.Lifetime != 99 {
		t.Fatalf("expected Priority/Lifetime 99/99, got %d/%d", model.Priority, model.Lifetime)
	}
	if model.Title != "" {
		t.Fatalf("expected empty Title, got %q", model.Title)
	}
	if model.Aux != "" {
		t.Fatalf("expected empty Aux, got %q", model.Aux)
	}
}
