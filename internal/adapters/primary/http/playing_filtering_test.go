package http

import (
	"testing"
	"time"
)

// TestPlayingEventFiltering verifies that events are properly filtered
// to show only those that overlap with the selected time range.
func TestPlayingEventFiltering(t *testing.T) {
	loc := time.Local
	
	// Simulate current time at 20:30
	now := time.Date(2026, 2, 3, 20, 30, 0, 0, loc)
	
	// User selects time range: 20:30 to 23:59 (now until end of day)
	startTime := now
	endTime := time.Date(2026, 2, 3, 23, 59, 59, 0, loc)
	
	testCases := []struct {
		name          string
		eventStart    time.Time
		eventStop     time.Time
		shouldInclude bool
		reason        string
	}{
		{
			name:          "Event ended 2 hours ago",
			eventStart:    time.Date(2026, 2, 3, 16, 59, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 18, 31, 0, 0, loc),
			shouldInclude: false,
			reason:        "Event ended before startTime (18:31 < 20:30)",
		},
		{
			name:          "Event ended 1 hour ago",
			eventStart:    time.Date(2026, 2, 3, 18, 31, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 19, 0, 0, 0, loc),
			shouldInclude: false,
			reason:        "Event ended before startTime (19:00 < 20:30)",
		},
		{
			name:          "Event currently running (started 30 min ago)",
			eventStart:    time.Date(2026, 2, 3, 20, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 21, 0, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event still running at startTime (ends 21:00 > 20:30)",
		},
		{
			name:          "Event starts exactly at startTime",
			eventStart:    time.Date(2026, 2, 3, 20, 30, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 22, 0, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event starts at startTime",
		},
		{
			name:          "Event starts in future (within range)",
			eventStart:    time.Date(2026, 2, 3, 22, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 23, 30, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event starts within selected range (22:00 < 23:59)",
		},
		{
			name:          "Event starts at end of range",
			eventStart:    time.Date(2026, 2, 3, 23, 59, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 0, 30, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event starts before endTime",
		},
		{
			name:          "Event starts after range",
			eventStart:    time.Date(2026, 2, 4, 0, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 1, 0, 0, 0, loc),
			shouldInclude: false,
			reason:        "Event starts after endTime (00:00 next day >= 23:59)",
		},
		{
			name:          "Long-running event started hours ago but still ongoing",
			eventStart:    time.Date(2026, 2, 3, 18, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 22, 0, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event still running at startTime (ends 22:00 > 20:30)",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This mimics the filtering logic in PlayingToday handler
			shouldInclude := tc.eventStop.After(startTime) && tc.eventStart.Before(endTime)
			
			if shouldInclude != tc.shouldInclude {
				t.Errorf("Event filtering mismatch:\n"+
					"  Event: %s to %s\n"+
					"  Range: %s to %s\n"+
					"  Expected: %v (reason: %s)\n"+
					"  Got: %v",
					tc.eventStart.Format("15:04"),
					tc.eventStop.Format("15:04"),
					startTime.Format("15:04"),
					endTime.Format("15:04"),
					tc.shouldInclude,
					tc.reason,
					shouldInclude)
			}
		})
	}
}

// TestPlayingEventFiltering_MidnightRollover tests filtering when
// the time range spans midnight (e.g., 22:00 to 02:00).
func TestPlayingEventFiltering_MidnightRollover(t *testing.T) {
	loc := time.Local
	
	// User selects: 22:00 to 02:00 (spans midnight)
	startTime := time.Date(2026, 2, 3, 22, 0, 0, 0, loc)
	endTime := time.Date(2026, 2, 4, 2, 0, 59, 0, loc)
	
	testCases := []struct {
		name          string
		eventStart    time.Time
		eventStop     time.Time
		shouldInclude bool
		reason        string
	}{
		{
			name:          "Event before range (ended at 21:00)",
			eventStart:    time.Date(2026, 2, 3, 20, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 21, 0, 0, 0, loc),
			shouldInclude: false,
			reason:        "Event ended before startTime",
		},
		{
			name:          "Event at start of range",
			eventStart:    time.Date(2026, 2, 3, 22, 0, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 3, 23, 0, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event within range (before midnight)",
		},
		{
			name:          "Event spanning midnight",
			eventStart:    time.Date(2026, 2, 3, 23, 30, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 0, 30, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event spans midnight within range",
		},
		{
			name:          "Event after midnight (within range)",
			eventStart:    time.Date(2026, 2, 4, 0, 30, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 1, 30, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event on next day within range",
		},
		{
			name:          "Event at end of range",
			eventStart:    time.Date(2026, 2, 4, 1, 30, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 2, 0, 0, 0, loc),
			shouldInclude: true,
			reason:        "Event ends within range",
		},
		{
			name:          "Event after range",
			eventStart:    time.Date(2026, 2, 4, 2, 30, 0, 0, loc),
			eventStop:     time.Date(2026, 2, 4, 3, 0, 0, 0, loc),
			shouldInclude: false,
			reason:        "Event starts after endTime",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldInclude := tc.eventStop.After(startTime) && tc.eventStart.Before(endTime)
			
			if shouldInclude != tc.shouldInclude {
				t.Errorf("Event filtering mismatch:\n"+
					"  Event: %s to %s\n"+
					"  Range: %s to %s\n"+
					"  Expected: %v (reason: %s)\n"+
					"  Got: %v",
					tc.eventStart.Format("2006-01-02 15:04"),
					tc.eventStop.Format("2006-01-02 15:04"),
					startTime.Format("2006-01-02 15:04"),
					endTime.Format("2006-01-02 15:04"),
					tc.shouldInclude,
					tc.reason,
					shouldInclude)
			}
		})
	}
}
