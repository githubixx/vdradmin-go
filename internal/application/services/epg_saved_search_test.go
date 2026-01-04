package services

import (
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestExecuteSavedEPGSearch_Phrase_CaseInsensitive(t *testing.T) {
	events := []domain.EPGEvent{
		{EventID: 1, ChannelID: "C-1-1-1", ChannelNumber: 1, ChannelName: "Ch1", Title: "Monaco Franze", Start: time.Unix(10, 0)},
		{EventID: 2, ChannelID: "C-2-2-2", ChannelNumber: 2, ChannelName: "Ch2", Title: "Other", Description: "MONACO FRANZE", Start: time.Unix(11, 0)},
	}
	order := map[string]int{"C-1-1-1": 1, "C-2-2-2": 2}

	search := config.EPGSearch{Pattern: "monaco franze", Mode: "phrase", MatchCase: false, InTitle: true, InDesc: true}
	got, err := ExecuteSavedEPGSearch(events, search, order)
	if err != nil {
		t.Fatalf("ExecuteSavedEPGSearch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
}

func TestExecuteSavedEPGSearch_Regex_ChannelRange(t *testing.T) {
	events := []domain.EPGEvent{
		{EventID: 1, ChannelID: "C-1", ChannelNumber: 1, ChannelName: "Ch1", Title: "abbey road", Start: time.Unix(10, 0)},
		{EventID: 2, ChannelID: "C-2", ChannelNumber: 2, ChannelName: "Ch2", Title: "abba", Start: time.Unix(11, 0)},
		{EventID: 3, ChannelID: "C-3", ChannelNumber: 3, ChannelName: "Ch3", Title: "abbey", Start: time.Unix(12, 0)},
	}
	order := map[string]int{"C-1": 1, "C-2": 2, "C-3": 3}

	search := config.EPGSearch{
		Pattern:     "^abb.*",
		Mode:        "regex",
		MatchCase:   true,
		InTitle:     true,
		UseChannel:  "range",
		ChannelFrom: "C-2",
		ChannelTo:   "C-3",
	}

	got, err := ExecuteSavedEPGSearch(events, search, order)
	if err != nil {
		t.Fatalf("ExecuteSavedEPGSearch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches (channels 2-3), got %d", len(got))
	}
	if got[0].ChannelID != "C-2" || got[1].ChannelID != "C-3" {
		t.Fatalf("unexpected channels: %q, %q", got[0].ChannelID, got[1].ChannelID)
	}
}
