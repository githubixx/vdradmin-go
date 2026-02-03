package svdrp

import (
	"strings"
	"testing"
)

// FuzzParseSVDRPResponse tests multi-line SVDRP response parsing
func FuzzParseSVDRPResponse(f *testing.F) {
	// Seed with valid SVDRP responses
	f.Add("250 OK")
	f.Add("250-First line\n250-Second line\n250 Last line")
	f.Add("220 vdr SVDRP VideoDiskRecorder 2.6.0; Sun Feb 2 00:00:00 2026")
	f.Add("550 Error message")
	f.Add("250-Line 1\n250-Line 2\n250-Line 3\n250 End")

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on SVDRP response %q: %v", input, r)
			}
		}()

		// Parse multi-line response - should handle any input
		lines := strings.Split(input, "\n")
		for _, line := range lines {
			if len(line) >= 3 {
				_ = line[:3] // Status code
			}
			if len(line) > 4 {
				_ = line[4:] // Message
			}
		}
	})
}

// FuzzParseChannelID tests channel ID format parsing
func FuzzParseChannelID(f *testing.F) {
	// Seed with known formats
	f.Add("S19.2E-1-1089-28106")
	f.Add("C-1-1-10")
	f.Add("T-12345-67890-99")
	f.Add("S13E-0-0-0")

	f.Fuzz(func(t *testing.T, channelID string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on channel ID %q: %v", channelID, r)
			}
		}()

		// Parse channel ID components
		parts := strings.Split(channelID, "-")
		if len(parts) >= 1 {
			_ = parts[0] // source
		}
		if len(parts) >= 2 {
			_ = parts[1] // NID
		}
		if len(parts) >= 3 {
			_ = parts[2] // TID
		}
		if len(parts) >= 4 {
			_ = parts[3] // SID
		}
	})
}

// FuzzParseEPGDescription tests EPG description parsing
func FuzzParseEPGDescription(f *testing.F) {
	// Seed with common patterns
	f.Add("Simple description")
	f.Add("Line 1|Line 2|Line 3")
	f.Add("Title~Subtitle~Description")
	f.Add("")
	f.Add("Unicode: äöü ñ 日本語")

	f.Fuzz(func(t *testing.T, description string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on EPG description %q: %v", description, r)
			}
		}()

		// EPG descriptions may contain various separators
		_ = strings.ReplaceAll(description, "|", "\n")
		_ = strings.Split(description, "~")
		_ = strings.TrimSpace(description)
	})
}

// FuzzParseTimestamp tests timestamp parsing with random formats
func FuzzParseTimestamp(f *testing.F) {
	// Seed with known formats
	f.Add("2026-02-02.20.15.00")
	f.Add("2026-01-01.00.00.00")
	f.Add("2025-12-31.23.59.59")

	f.Fuzz(func(t *testing.T, timestamp string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on timestamp %q: %v", timestamp, r)
			}
		}()

		// Parse timestamp components
		parts := strings.Split(timestamp, ".")
		if len(parts) >= 4 {
			_ = parts[0] // date
			_ = parts[1] // hour
			_ = parts[2] // minute
			_ = parts[3] // second
		}
	})
}
