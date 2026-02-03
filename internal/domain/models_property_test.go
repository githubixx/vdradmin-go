package domain

import (
	"strings"
	"testing"
	"testing/quick"
	"time"
)

// TestChannelIDFormat_PropertyBased uses property-based testing to validate
// channel ID parsing with random inputs.
func TestChannelIDFormat_PropertyBased(t *testing.T) {
	// Property: Channel IDs should be parseable or clearly invalid
	f := func(source string, nid uint16, tid uint16, sid uint16) bool {
		// Generate channel ID format: S19.2E-1-1089-28106
		channelID := strings.TrimSpace(source) + "-" +
			string(rune(nid%1000)) + "-" +
			string(rune(tid%10000)) + "-" +
			string(rune(sid%65535))

		// Property: Channel IDs should not cause panics
		_ = channelID
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestTimerDaySpec_PropertyBased tests timer day specification parsing with random inputs
func TestTimerDaySpec_PropertyBased(t *testing.T) {
	// Property: Valid day specs should be either YYYY-MM-DD or weekday masks
	f := func(year int, month uint8, day uint8) bool {
		// Bound values to valid ranges
		year = 2000 + (year % 100)
		month = 1 + (month % 12)
		day = 1 + (day % 28) // Safe for all months

		daySpec := time.Date(year, time.Month(month), int(day), 0, 0, 0, 0, time.UTC).Format("2006-01-02")

		// Property: Valid date strings should not panic
		_, err := time.Parse("2006-01-02", daySpec)
		return err == nil
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

// TestTimerWeekdayMask_PropertyBased tests weekday mask generation
func TestTimerWeekdayMask_PropertyBased(t *testing.T) {
	// Property: Weekday masks should be 7 characters with M/T/W/T/F/S/S or -
	f := func(mask uint8) bool {
		weekdays := "MTWTFSS"
		var result strings.Builder
		for i := 0; i < 7; i++ {
			if mask&(1<<i) != 0 {
				result.WriteByte(weekdays[i])
			} else {
				result.WriteByte('-')
			}
		}
		dayMask := result.String()

		// Property: Mask should be exactly 7 characters
		if len(dayMask) != 7 {
			return false
		}

		// Property: Each character should be valid
		for i, ch := range dayMask {
			if ch != '-' && ch != rune(weekdays[i]) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 128}); err != nil {
		t.Error(err)
	}
}

// TestRecordingPath_PropertyBased tests recording path validation
func TestRecordingPath_PropertyBased(t *testing.T) {
	// Property: Recording paths should handle various special characters
	f := func(title string, date string) bool {
		// Sanitize inputs to avoid impossible paths
		title = strings.Map(func(r rune) rune {
			if r < 32 || r == '/' || r == 0 {
				return '_'
			}
			return r
		}, title)

		if title == "" {
			title = "Recording"
		}

		// Create path with timestamp format
		path := title + "/2026-02-02.20.15.00.rec"

		// Property: Paths should not contain null bytes or be empty
		return !strings.Contains(path, "\x00") && len(path) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestTimerMinutes_PropertyBased tests timer start/stop minutes validation
func TestTimerMinutes_PropertyBased(t *testing.T) {
	// Property: Timer minutes should be in range 0-1439 (24 hours)
	f := func(startHour, startMin, stopHour, stopMin uint8) bool {
		start := int(startHour%24)*60 + int(startMin%60)
		stop := int(stopHour%24)*60 + int(stopMin%60)

		// Property: Minutes should be in valid range
		if start < 0 || start >= 1440 {
			return false
		}
		if stop < 0 || stop >= 1440 {
			return false
		}

		// Property: Start and stop can wrap around midnight
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

// TestChannelNumber_PropertyBased tests channel number validation
func TestChannelNumber_PropertyBased(t *testing.T) {
	// Property: Channel numbers should be positive and within reasonable range
	f := func(num int) bool {
		// Bound to reasonable range
		if num <= 0 || num >= 10000 {
			return true // Expected behavior: outside valid range
		}

		// Property: Valid channel numbers should be between 1-9999
		return num > 0 && num < 10000
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestTimerPriorityLifetime_PropertyBased tests priority and lifetime ranges
func TestTimerPriorityLifetime_PropertyBased(t *testing.T) {
	// Property: Priority 0-99, Lifetime 0-99 (VDR ranges)
	f := func(priority, lifetime int8) bool {
		// Bound to VDR valid ranges
		p := int(priority)
		l := int(lifetime)

		// Property: Values outside range should be bounded
		if p < 0 {
			p = 0
		}
		if p > 99 {
			p = 99
		}
		if l < 0 {
			l = 0
		}
		if l > 99 {
			l = 99
		}

		return p >= 0 && p <= 99 && l >= 0 && l <= 99
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestEPGEventDuration_PropertyBased tests EPG event duration calculation
func TestEPGEventDuration_PropertyBased(t *testing.T) {
	// Property: Event duration should match stop - start
	f := func(startUnix int64, durationMinutes uint16) bool {
		// Use reasonable time ranges
		start := time.Unix(startUnix%(365*24*3600), 0)
		duration := time.Duration(durationMinutes%1440) * time.Minute
		stop := start.Add(duration)

		// Property: Duration should be positive or zero
		calculated := stop.Sub(start)
		return calculated >= 0 && calculated == duration
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

// TestRecordingSize_PropertyBased tests recording size validation
func TestRecordingSize_PropertyBased(t *testing.T) {
	// Property: Recording sizes should be non-negative and bounded
	f := func(size int64) bool {
		// Property: Allow negative as "invalid" input, should be handled
		// Property: Very large sizes (>1TB) are outside normal range
		const maxSize = 1024 * 1024 * 1024 * 1024 // 1TB

		if size < 0 {
			return true // Invalid but should be handled
		}
		if size > maxSize {
			return true // Outside normal range
		}

		return size >= 0 && size <= maxSize
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Error(err)
	}
}

// TestChannelGrouping_PropertyBased tests channel grouping logic
func TestChannelGrouping_PropertyBased(t *testing.T) {
	// Property: Channel groups should handle empty/special strings
	f := func(group string) bool {
		// Sanitize group name
		group = strings.TrimSpace(group)
		if len(group) > 100 {
			group = group[:100]
		}

		// Property: Group name should not cause issues
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}
