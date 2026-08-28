package services

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

func TestEPGService_SearchEPGWithCriteria_FiltersAndLimits(t *testing.T) {
	start := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	service := NewEPGService(ports.NewMockVDRClient().
		WithChannels([]domain.Channel{{ID: "one", Number: 1, Name: "One"}, {ID: "two", Number: 2, Name: "Two"}}).
		WithEPGEvents([]domain.EPGEvent{
			{EventID: 1, ChannelID: "one", ChannelNumber: 1, Title: "News", Start: start, Stop: start.Add(time.Hour)},
			{EventID: 2, ChannelID: "two", ChannelNumber: 2, Title: "Evening news", Start: start.Add(time.Hour), Stop: start.Add(2 * time.Hour)},
			{EventID: 3, ChannelID: "two", ChannelNumber: 2, Title: "News review", Start: start.Add(2 * time.Hour), Stop: start.Add(3 * time.Hour)},
		}), time.Minute)

	result, err := service.SearchEPGWithCriteria(context.Background(), EPGSearchCriteria{
		Pattern:     "news",
		InTitle:     true,
		ChannelIDs:  []string{"two"},
		StartsAt:    timePtr(start.Add(90 * time.Minute)),
		ResultLimit: 1,
	})
	if err != nil {
		t.Fatalf("SearchEPGWithCriteria: %v", err)
	}
	if result.Total != 2 || !result.Truncated || len(result.Events) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Events[0].EventID != 2 {
		t.Fatalf("expected earliest matching event, got %d", result.Events[0].EventID)
	}
}

func TestEPGService_SearchEPGWithCriteria_ValidatesCriteria(t *testing.T) {
	service := NewEPGService(ports.NewMockVDRClient(), time.Minute)
	_, err := service.SearchEPGWithCriteria(context.Background(), EPGSearchCriteria{Pattern: "", ResultLimit: 1})
	if err == nil {
		t.Fatal("expected missing pattern validation error")
	}

	_, err = service.SearchEPGWithCriteria(context.Background(), EPGSearchCriteria{Pattern: "news", Mode: "glob"})
	if err == nil {
		t.Fatal("expected invalid mode validation error")
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
