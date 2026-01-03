package http

import (
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// criticalTimerIDs is a small helper used by tests and overlap highlighting.
// It returns the set of timers that are in a critical overlap state (red).
func criticalTimerIDs(timers []domain.Timer, dvbCards int, transponderKey func(domain.Timer) string) map[int]bool {
	from, to := overlapWindowFromTimers(timers)
	_, critical := timerOverlapStates(timers, dvbCards, from, to, transponderKey)
	return critical
}

func overlapWindowFromTimers(timers []domain.Timer) (time.Time, time.Time) {
	var from time.Time
	var to time.Time
	for _, t := range timers {
		if t.Start.IsZero() || t.Stop.IsZero() {
			continue
		}
		if from.IsZero() || t.Start.Before(from) {
			from = t.Start
		}
		if to.IsZero() || t.Stop.After(to) {
			to = t.Stop
		}
	}
	if from.IsZero() {
		now := time.Now()
		from = now.Add(-24 * time.Hour)
		to = now.Add(8 * 24 * time.Hour)
	}
	return from, to
}
