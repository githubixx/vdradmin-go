package domain

import (
	"testing"
	"time"
)

// TestChannel tests the Channel struct
func TestChannel(t *testing.T) {
	t.Run("ValidChannel", func(t *testing.T) {
		ch := Channel{
			ID:     "S19.2E-1-1089-28106",
			Number: 1,
			Name:   "Das Erste HD",
		}

		if ch.ID == "" {
			t.Error("Channel ID should not be empty")
		}
		if ch.Number <= 0 {
			t.Error("Channel number should be positive")
		}
		if ch.Name == "" {
			t.Error("Channel name should not be empty")
		}
	})

	t.Run("MinimalChannel", func(t *testing.T) {
		ch := Channel{
			ID:       "test-id",
			Number:   1,
			Name:     "Test Channel",
			Provider: "",
			Freq:     "",
		}

		if ch.ID != "test-id" || ch.Number != 1 || ch.Name != "Test Channel" {
			t.Error("Minimal channel fields should be preserved")
		}
		if ch.Provider != "" || ch.Freq != "" {
			t.Error("Unset fields should be empty strings")
		}
	})

	t.Run("ZeroValueChannel", func(t *testing.T) {
		var ch Channel
		if ch.Number != 0 {
			t.Error("Zero value channel should have number 0")
		}
		if ch.ID != "" || ch.Name != "" {
			t.Error("Zero value channel should have empty strings")
		}
	})
}

// TestEPGEvent tests the EPGEvent struct
func TestEPGEvent(t *testing.T) {
	t.Run("ValidEvent", func(t *testing.T) {
		start := time.Date(2026, 2, 2, 20, 15, 0, 0, time.UTC)

		event := EPGEvent{
			EventID:  12345,
			Start:    start,
			Duration: 90 * time.Minute,
		}

		if event.EventID != 12345 {
			t.Errorf("Expected EventID 12345, got %d", event.EventID)
		}
		if event.Duration != 90*time.Minute {
			t.Errorf("Expected duration 90min, got %v", event.Duration)
		}
		if !event.Start.Equal(start) {
			t.Error("Start time should match")
		}
	})

	t.Run("EventWithVideoInfo", func(t *testing.T) {
		event := EPGEvent{
			Video: VideoInfo{
				Format: "16:9",
				HD:     true,
			},
		}

		if !event.Video.HD {
			t.Error("HD flag should be true")
		}
		if event.Video.Format != "16:9" {
			t.Errorf("Expected format 16:9, got %s", event.Video.Format)
		}
	})

	t.Run("EventWithAudioInfo", func(t *testing.T) {
		event := EPGEvent{
			Audio: []AudioInfo{
				{Language: "de", Channels: 2},
				{Language: "en", Channels: 2},
				{Language: "de", Channels: 6},
			},
		}

		if len(event.Audio) != 3 {
			t.Errorf("Expected 3 audio tracks, got %d", len(event.Audio))
		}
		if event.Audio[2].Channels != 6 {
			t.Error("Third audio track should be 5.1 (6 channels)")
		}
	})

	t.Run("EventWithVPS", func(t *testing.T) {
		vps := time.Date(2026, 2, 2, 20, 15, 0, 0, time.UTC)
		event := EPGEvent{
			VPS: &vps,
		}

		if event.VPS == nil {
			t.Error("VPS should not be nil")
		}
		if !event.VPS.Equal(vps) {
			t.Error("VPS time should match")
		}
	})

	t.Run("EventWithoutVPS", func(t *testing.T) {
		event := EPGEvent{
			VPS: nil,
		}

		if event.VPS != nil {
			t.Error("VPS should be nil")
		}
	})
}

// TestVideoInfo tests video stream information
func TestVideoInfo(t *testing.T) {
	tests := []struct {
		name   string
		info   VideoInfo
		wantHD bool
	}{
		{"HD 16:9", VideoInfo{Format: "16:9", HD: true}, true},
		{"SD 16:9", VideoInfo{Format: "16:9", HD: false}, false},
		{"SD 4:3", VideoInfo{Format: "4:3", HD: false}, false},
		{"Empty", VideoInfo{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.info.HD != tt.wantHD {
				t.Errorf("HD flag: got %v, want %v", tt.info.HD, tt.wantHD)
			}
		})
	}
}

// TestAudioInfo tests audio stream information
func TestAudioInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     AudioInfo
		wantLang string
		wantCh   int
	}{
		{"Mono DE", AudioInfo{Language: "de", Channels: 1}, "de", 1},
		{"Stereo EN", AudioInfo{Language: "en", Channels: 2}, "en", 2},
		{"5.1 DE", AudioInfo{Language: "de", Channels: 6}, "de", 6},
		{"Empty", AudioInfo{}, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.info.Language != tt.wantLang {
				t.Errorf("Language: got %s, want %s", tt.info.Language, tt.wantLang)
			}
			if tt.info.Channels != tt.wantCh {
				t.Errorf("Channels: got %d, want %d", tt.info.Channels, tt.wantCh)
			}
		})
	}
}

// TestTimer tests the Timer struct
func TestTimer(t *testing.T) {
	t.Run("ValidTimer", func(t *testing.T) {
		timer := Timer{
			ID:           1,
			Active:       true,
			DaySpec:      "2026-02-02",
			StartMinutes: 1215, // 20:15
		}

		if timer.ID != 1 {
			t.Errorf("Expected ID 1, got %d", timer.ID)
		}
		if !timer.Active {
			t.Error("Timer should be active")
		}
		if timer.StartMinutes != 1215 {
			t.Errorf("Expected start 1215 minutes, got %d", timer.StartMinutes)
		}
		if timer.DaySpec != "2026-02-02" {
			t.Errorf("Expected DaySpec 2026-02-02, got %s", timer.DaySpec)
		}
	})

	t.Run("RecurringTimer", func(t *testing.T) {
		timer := Timer{
			DaySpec:  "MTWTF--", // Monday to Friday
			Lifetime: 7,
		}

		if timer.DaySpec != "MTWTF--" {
			t.Errorf("Recurring timer DaySpec: got %s, want MTWTF--", timer.DaySpec)
		}
		if timer.Lifetime != 7 {
			t.Errorf("Lifetime: got %d, want 7", timer.Lifetime)
		}
	})

	t.Run("InactiveTimer", func(t *testing.T) {
		timer := Timer{
			Active: false,
		}

		if timer.Active {
			t.Error("Timer should be inactive")
		}
	})

	t.Run("TimerWithAux", func(t *testing.T) {
		timer := Timer{
			Aux: "<epgsearch>searchpattern=test</epgsearch>",
		}

		if timer.Aux == "" {
			t.Error("Aux field should not be empty")
		}
		if timer.Aux != "<epgsearch>searchpattern=test</epgsearch>" {
			t.Errorf("Aux field incorrect: got %s", timer.Aux)
		}
	})
}

// TestRecording tests the Recording struct
func TestRecording(t *testing.T) {
	t.Run("ValidRecording", func(t *testing.T) {
		rec := Recording{
			Path:     "Tagesschau/2026-02-01.20.15.50-99.rec",
			DiskPath: "/hdd01/vdr/video/Tagesschau/2026-02-01.20.15.50-99.rec",
			Title:    "Tagesschau",
			Size:     1024 * 1024 * 1024, // 1GB
		}

		if rec.Path == "" {
			t.Error("Path should not be empty")
		}
		if rec.DiskPath == "" {
			t.Error("DiskPath should not be empty")
		}
		if rec.Title != "Tagesschau" {
			t.Errorf("Expected title Tagesschau, got %s", rec.Title)
		}
		if rec.Size != 1024*1024*1024 {
			t.Errorf("Size mismatch: got %d", rec.Size)
		}
	})

	t.Run("FolderRecording", func(t *testing.T) {
		rec := Recording{
			IsFolder: true,
			Children: []*Recording{
				{},
				{},
			},
		}

		if !rec.IsFolder {
			t.Error("Recording should be marked as folder")
		}
		if len(rec.Children) != 2 {
			t.Errorf("Expected 2 children, got %d", len(rec.Children))
		}
	})

	t.Run("RecordingWithoutDiskPath", func(t *testing.T) {
		rec := Recording{
			DiskPath: "",
		}

		if rec.DiskPath != "" {
			t.Error("DiskPath should be empty when not resolved")
		}
	})

	t.Run("EmptyRecording", func(t *testing.T) {
		var rec Recording
		if rec.IsFolder {
			t.Error("Zero value recording should not be folder")
		}
		if rec.Size != 0 {
			t.Error("Zero value recording should have size 0")
		}
		if len(rec.Children) != 0 {
			t.Error("Zero value recording should have no children")
		}
	})
}

// TestAutoTimer tests the AutoTimer struct
func TestAutoTimer(t *testing.T) {
	t.Run("ValidAutoTimer", func(t *testing.T) {
		at := AutoTimer{
			Pattern:       "Tagesschau",
			UseRegex:      false,
			SearchIn:      SearchTitle,
			ChannelFilter: []string{"Das Erste HD", "ZDF HD"},
			DayOfWeek:     []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
			Done:          []int{1001, 1002},
		}

		if at.Pattern != "Tagesschau" {
			t.Errorf("Pattern: got %s, want Tagesschau", at.Pattern)
		}
		if at.UseRegex {
			t.Error("UseRegex should be false")
		}
		if at.SearchIn != SearchTitle {
			t.Errorf("SearchIn: got %v, want %v", at.SearchIn, SearchTitle)
		}
		if len(at.ChannelFilter) != 2 {
			t.Errorf("Expected 2 channels, got %d", len(at.ChannelFilter))
		}
		if len(at.DayOfWeek) != 5 {
			t.Errorf("Expected 5 days, got %d", len(at.DayOfWeek))
		}
		if len(at.Done) != 2 {
			t.Errorf("Expected 2 done events, got %d", len(at.Done))
		}
	})

	t.Run("RegexAutoTimer", func(t *testing.T) {
		at := AutoTimer{
			UseRegex: true,
			SearchIn: SearchTitleSubtitle,
		}

		if !at.UseRegex {
			t.Error("UseRegex should be true")
		}
		if at.SearchIn != SearchTitleSubtitle {
			t.Error("SearchIn should be SearchTitleSubtitle")
		}
	})

	t.Run("InactiveAutoTimer", func(t *testing.T) {
		at := AutoTimer{
			Active: false,
		}

		if at.Active {
			t.Error("AutoTimer should be inactive")
		}
	})

	t.Run("AutoTimerWithoutTimeRestriction", func(t *testing.T) {
		at := AutoTimer{
			TimeStart: nil,
			TimeEnd:   nil,
		}

		if at.TimeStart != nil {
			t.Error("TimeStart should be nil")
		}
		if at.TimeEnd != nil {
			t.Error("TimeEnd should be nil")
		}
	})

	t.Run("AutoTimerSearchAll", func(t *testing.T) {
		at := AutoTimer{
			SearchIn: SearchAll,
		}

		if at.SearchIn != SearchAll {
			t.Error("SearchIn should be SearchAll")
		}
	})
}

// TestSearchScope tests the SearchScope constants
func TestSearchScope(t *testing.T) {
	tests := []struct {
		name  string
		scope SearchScope
		value int
	}{
		{"SearchTitle", SearchTitle, 0},
		{"SearchTitleSubtitle", SearchTitleSubtitle, 1},
		{"SearchAll", SearchAll, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.scope) != tt.value {
				t.Errorf("Expected value %d, got %d", tt.value, int(tt.scope))
			}
		})
	}
}

// TestRecordingNested tests nested recording structures
func TestRecordingNested(t *testing.T) {
	t.Run("NestedFolders", func(t *testing.T) {
		root := Recording{
			IsFolder: true,
			Children: []*Recording{
				{
					IsFolder: true,
					Children: []*Recording{
						{IsFolder: false},
						{IsFolder: false},
					},
				},
				{
					IsFolder: true,
					Children: []*Recording{
						{IsFolder: false},
					},
				},
			},
		}

		if !root.IsFolder {
			t.Error("Root should be a folder")
		}
		if len(root.Children) != 2 {
			t.Errorf("Root should have 2 children, got %d", len(root.Children))
		}
		if len(root.Children[0].Children) != 2 {
			t.Errorf("Show1 should have 2 episodes, got %d", len(root.Children[0].Children))
		}
		if root.Children[0].Children[0].IsFolder {
			t.Error("Episode should not be a folder")
		}
	})
}

// TestTimerMinutesConversion tests StartMinutes/StopMinutes edge cases
func TestTimerMinutesConversion(t *testing.T) {
	tests := []struct {
		name         string
		startMinutes int
		stopMinutes  int
		wantStart    string // "HH:MM"
		wantStop     string // "HH:MM"
	}{
		{"Midnight", 0, 60, "00:00", "01:00"},
		{"Morning", 360, 420, "06:00", "07:00"},
		{"PrimeTime", 1200, 1320, "20:00", "22:00"},
		{"LateNight", 1380, 1440, "23:00", "24:00"},
		{"SpansMidnight", 1420, 80, "23:40", "01:20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timer := Timer{
				StartMinutes: tt.startMinutes,
				StopMinutes:  tt.stopMinutes,
			}

			// Verify minutes are stored correctly
			if timer.StartMinutes != tt.startMinutes {
				t.Errorf("StartMinutes: got %d, want %d", timer.StartMinutes, tt.startMinutes)
			}
			if timer.StopMinutes != tt.stopMinutes {
				t.Errorf("StopMinutes: got %d, want %d", timer.StopMinutes, tt.stopMinutes)
			}

			// Verify hour/minute calculation
			startHour := timer.StartMinutes / 60
			startMin := timer.StartMinutes % 60
			if startHour*60+startMin != tt.startMinutes {
				t.Error("Start time calculation mismatch")
			}
		})
	}
}

// TestEPGEventDuration tests event duration calculation consistency
func TestEPGEventDuration(t *testing.T) {
	t.Run("DurationMatchesStartStop", func(t *testing.T) {
		start := time.Date(2026, 2, 2, 20, 0, 0, 0, time.UTC)
		stop := time.Date(2026, 2, 2, 22, 30, 0, 0, time.UTC)

		event := EPGEvent{
			Duration: stop.Sub(start),
		}

		expectedDuration := 150 * time.Minute
		if event.Duration != expectedDuration {
			t.Errorf("Duration: got %v, want %v", event.Duration, expectedDuration)
		}
	})

	t.Run("DurationSpansMidnight", func(t *testing.T) {
		start := time.Date(2026, 2, 2, 23, 30, 0, 0, time.UTC)
		stop := time.Date(2026, 2, 3, 1, 0, 0, 0, time.UTC)

		event := EPGEvent{
			Duration: stop.Sub(start),
		}

		expectedDuration := 90 * time.Minute
		if event.Duration != expectedDuration {
			t.Errorf("Duration: got %v, want %v", event.Duration, expectedDuration)
		}
	})
}
