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

	if startTimeStr != "14:30" {
		t.Errorf("Expected start time string '14:30', got: %s", startTimeStr)
	}

	if endTimeStr != "23:59" {
		t.Errorf("Expected end time string '23:59', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := time.Date(2026, 2, 3, 23, 59, 59, 0, loc)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_CustomTimes(t *testing.T) {
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=18:00&end=22:30", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if startTimeStr != "18:00" {
		t.Errorf("Expected start time string '18:00', got: %s", startTimeStr)
	}

	if endTimeStr != "22:30" {
		t.Errorf("Expected end time string '22:30', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 18, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := time.Date(2026, 2, 3, 22, 30, 59, 0, loc)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
	}
}

func TestParsePlayingTimeParams_EndBeforeStart(t *testing.T) {
	// When end time is before start time, it should be treated as next day
	loc := time.Local
	dayStart := time.Date(2026, 2, 3, 0, 0, 0, 0, loc)
	localNow := time.Date(2026, 2, 3, 14, 30, 0, 0, loc)

	req := httptest.NewRequest(http.MethodGet, "/playing?start=22:00&end=02:00", nil)

	startTimeStr, endTimeStr, startTime, endTime, err := parsePlayingTimeParams(req, dayStart, localNow)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if startTimeStr != "22:00" {
		t.Errorf("Expected start time string '22:00', got: %s", startTimeStr)
	}

	if endTimeStr != "02:00" {
		t.Errorf("Expected end time string '02:00', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 3, 22, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	// End time should be on the next day
	expectedEnd := time.Date(2026, 2, 4, 2, 0, 59, 0, loc)
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
	if startTimeStr != "14:30" {
		t.Errorf("Expected fallback start time '14:30', got: %s", startTimeStr)
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
	if endTimeStr != "23:59" {
		t.Errorf("Expected fallback end time '23:59', got: %s", endTimeStr)
	}
}

func TestParsePlayingTimeParams_DifferentDay(t *testing.T) {
	// When viewing a different day, default start should be day start
	loc := time.Local
	dayStart := time.Date(2026, 2, 5, 0, 0, 0, 0, loc)  // Future day
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

	if endTimeStr != "23:59" {
		t.Errorf("Expected end time string '23:59', got: %s", endTimeStr)
	}

	expectedStart := time.Date(2026, 2, 5, 0, 0, 0, 0, loc)
	if !startTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got: %v", expectedStart, startTime)
	}

	expectedEnd := time.Date(2026, 2, 5, 23, 59, 59, 0, loc)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("Expected end time %v, got: %v", expectedEnd, endTime)
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
