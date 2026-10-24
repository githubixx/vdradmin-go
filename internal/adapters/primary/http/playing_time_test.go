package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParsePlayingTimeParams_DefaultValues(t *testing.T) {
	// Test default values when no query parameters are provided
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if startTimeStr != "14:00" {
		t.Errorf("Expected start time string '14:00', got: %s", startTimeStr)
	}

	if endTimeStr != "24:00" {
		t.Errorf("Expected end time string '24:00', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 14, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := dayStart.AddDate(0, 0, 1)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_CustomTimes(t *testing.T) {
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=18:00&end=22:00", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if startTimeStr != "18:00" {
		t.Errorf("Expected start time string '18:00', got: %s", startTimeStr)
	}

	if endTimeStr != "22:00" {
		t.Errorf("Expected end time string '22:00', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 18, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := time.Date(2026, 2, 3, 22, 0, 0, 0, loc)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_EndBeforeStart(t *testing.T) {
	// Reversed ranges must not silently become overnight windows.
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=22:00&end=02:00", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err == nil || err.Error() != "end time must be at least one hour after start time" {
		t.Errorf("Expected interval error, got: %v", err)
	}

	if startTimeStr != "22:00" {
		t.Errorf("Expected start time string '22:00', got: %s", startTimeStr)
	}

	if endTimeStr != "23:00" {
		t.Errorf("Expected corrected end time string '23:00', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 22, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := time.Date(2026, 2, 3, 23, 0, 0, 0, loc)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_InvalidStartTime(t *testing.T) {
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=invalid&end=22:00", nil)

	startTimeStr, endTimeStr, _, _, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err == nil {
		t.Error("Expected error for invalid start time, got nil")
	}

	if err.Error() != "invalid start time (expected HH:MM)" {
		t.Errorf("Expected error message 'invalid start time (expected HH:MM)', got: %v", err)
	}

	// Should fall back to default start time
	if startTimeStr != "14:00" {
		t.Errorf("Expected fallback start time '14:00', got: %s", startTimeStr)
	}

	// End time should still be parsed correctly
	if endTimeStr != "22:00" {
		t.Errorf("Expected end time '22:00', got: %s", endTimeStr)
	}
}

func TestParsePlayingTimeParams_InvalidEndTime(t *testing.T) {
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=18:00&end=invalid", nil)

	startTimeStr, endTimeStr, _, _, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err == nil {
		t.Error("Expected error for invalid end time, got nil")
	}

	if err.Error() != "invalid end time (expected HH:MM)" {
		t.Errorf("Expected error message 'invalid end time (expected HH:MM)', got: %v", err)
	}

	// Start time should still be parsed correctly
	if startTimeStr != "18:00" {
		t.Errorf("Expected start time '18:00', got: %s", startTimeStr)
	}

	// Should fall back to default end time
	if endTimeStr != "24:00" {
		t.Errorf("Expected fallback end time '24:00', got: %s", endTimeStr)
	}
}

func TestParsePlayingTimeParams_DifferentDay(t *testing.T) {
	// When viewing a different day, default start should be day start
	loc := time.Local
	dayStart := time.Date(2026, 2, 5, 0, 0, 0, 0, loc)   // Future day
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc) // Current time is earlier

	req := httptest.NewRequest(http.MethodGet, "/playing", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Should default to midnight (start of the viewed day)
	if startTimeStr != "00:00" {
		t.Errorf("Expected start time string '00:00' for different day, got: %s", startTimeStr)
	}

	if endTimeStr != "24:00" {
		t.Errorf("Expected end time string '24:00', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 5, 0, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := dayStart.AddDate(0, 0, 1)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_HourlyBoundaries(t *testing.T) {
	day := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		query, start, end string
		wantErr           bool
	}{
		{"start=07:00&end=08:00", "07:00", "08:00", false},
		{"start=07:00&end=09:00", "07:00", "09:00", false},
		{"start=07:00&end=07:00", "07:00", "08:00", true},
		{"start=07:00&end=06:00", "07:00", "08:00", true},
		{"start=23:00&end=24:00", "23:00", "24:00", false},
		{"start=23:00&end=00:00", "23:00", "24:00", true},
		{"start=00:00&end=24:00", "00:00", "24:00", false},
		{"start=07:09&end=23:59", "07:00", "24:00", true},
		{"start=07:59&end=08:01", "07:00", "09:00", true},
		{"start=24:00&end=25:00", "14:00", "24:00", true},
		{"", "14:00", "24:00", false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/playing?"+tc.query, nil)
			start, end, from, to, err := parsePlayingTimeParams(req, day, day.Add(14*time.Hour+30*time.Minute))
			if start != tc.start || end != tc.end || (err != nil) != tc.wantErr {
				t.Fatalf("got (%s, %s, %v), want (%s, %s, error=%t)", start, end, err, tc.start, tc.end, tc.wantErr)
			}
			if to.Sub(from) < time.Hour || to.After(day.AddDate(0, 0, 1)) {
				t.Fatalf("invalid same-day interval: %v to %v", from, to)
			}
		})
	}
}

func TestParsePlayingTimeParams_MidnightOnDSTDays(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []struct {
		month time.Month
		day   int
		hours time.Duration
	}{{time.March, 29, 23}, {time.October, 25, 25}} {
		day := time.Date(2026, date.month, date.day, 0, 0, 0, 0, loc)
		req := httptest.NewRequest(http.MethodGet, "/playing?start=00:00&end=24:00", nil)
		_, _, from, to, err := parsePlayingTimeParams(req, day, day)
		if err != nil || !to.Equal(day.AddDate(0, 0, 1)) || to.Sub(from) != date.hours*time.Hour {
			t.Fatalf("incorrect DST boundary: %v to %v, error %v", from, to, err)
		}
	}
}

func TestIsSameDay(t *testing.T) {
	loc := time.Local

	t.Run("SameDay", func(t *testing.T) {
		t1 := time.Date(2026, 2, 3, 10, 30, 0, 0, loc)
		t2 := time.Date(2026, 2, 3, 18, 45, 0, 0, loc)
		if !isSameDay(t1, t2) {
			t.Error("Expected times to be on same day")
		}
	})

	t.Run("DifferentDay", func(t *testing.T) {
		t1 := time.Date(2026, 2, 3, 10, 30, 0, 0, loc)
		t2 := time.Date(2026, 2, 4, 10, 30, 0, 0, loc)
		if isSameDay(t1, t2) {
			t.Error("Expected times to be on different days")
		}
	})

	t.Run("DifferentMonth", func(t *testing.T) {
		t1 := time.Date(2026, 2, 3, 10, 30, 0, 0, loc)
		t2 := time.Date(2026, 3, 3, 10, 30, 0, 0, loc)
		if isSameDay(t1, t2) {
			t.Error("Expected times to be on different days")
		}
	})

	t.Run("DifferentYear", func(t *testing.T) {
		t1 := time.Date(2026, 2, 3, 10, 30, 0, 0, loc)
		t2 := time.Date(2027, 2, 3, 10, 30, 0, 0, loc)
		if isSameDay(t1, t2) {
			t.Error("Expected times to be on different days")
		}
	})
}
