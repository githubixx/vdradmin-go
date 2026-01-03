package http

import (
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestHandler_timerDefaults_FallbackWhenConfigNil(t *testing.T) {
	h := &Handler{cfg: nil}
	p, l, ms, me := h.timerDefaults()
	if p != 50 || l != 99 || ms != 2 || me != 10 {
		t.Fatalf("expected defaults 50/99/2/10, got %d/%d/%d/%d", p, l, ms, me)
	}
}

func TestHandler_timerDefaults_UsesConfigValues(t *testing.T) {
	h := &Handler{cfg: &config.Config{Timer: config.TimerConfig{
		DefaultPriority:    12,
		DefaultLifetime:    34,
		DefaultMarginStart: 5,
		DefaultMarginEnd:   6,
	}}}
	p, l, ms, me := h.timerDefaults()
	if p != 12 || l != 34 || ms != 5 || me != 6 {
		t.Fatalf("expected 12/34/5/6, got %d/%d/%d/%d", p, l, ms, me)
	}
}

func TestHandler_timerDefaults_AllowsZeroMargins(t *testing.T) {
	h := &Handler{cfg: &config.Config{Timer: config.TimerConfig{
		DefaultPriority:    0,
		DefaultLifetime:    0,
		DefaultMarginStart: 0,
		DefaultMarginEnd:   0,
	}}}
	p, l, ms, me := h.timerDefaults()
	if p != 0 || l != 0 || ms != 0 || me != 0 {
		t.Fatalf("expected 0/0/0/0, got %d/%d/%d/%d", p, l, ms, me)
	}
}
