package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

const (
	defaultEPGSearchLimit = 50
	maxEPGSearchLimit     = 200
)

// EPGSearchCriteria describes an ad-hoc EPG search independent of any transport.
type EPGSearchCriteria struct {
	Pattern     string
	Mode        string
	MatchCase   bool
	InTitle     bool
	InSubtitle  bool
	InDesc      bool
	ChannelIDs  []string
	StartsAt    *time.Time
	EndsAt      *time.Time
	ResultLimit int
}

// EPGSearchResult contains sorted matching events and indicates whether a limit was applied.
type EPGSearchResult struct {
	Events    []domain.EPGEvent
	Total     int
	Truncated bool
}

// SearchEPGWithCriteria searches cached VDR EPG data using the supplied criteria.
func (s *EPGService) SearchEPGWithCriteria(ctx context.Context, criteria EPGSearchCriteria) (EPGSearchResult, error) {
	if err := validateEPGSearchCriteria(criteria); err != nil {
		return EPGSearchResult{}, err
	}

	events, err := s.GetEPG(ctx, "", time.Time{})
	if err != nil {
		return EPGSearchResult{}, err
	}

	channels, err := s.GetChannels(ctx)
	if err != nil {
		return EPGSearchResult{}, err
	}
	channelOrder := make(map[string]int, len(channels))
	for index, channel := range channels {
		channelOrder[channel.ID] = index + 1
	}

	matches, err := ExecuteSavedEPGSearch(events, config.EPGSearch{
		Pattern:    criteria.Pattern,
		Mode:       criteria.Mode,
		MatchCase:  criteria.MatchCase,
		InTitle:    criteria.InTitle,
		InSubtitle: criteria.InSubtitle,
		InDesc:     criteria.InDesc,
	}, channelOrder)
	if err != nil {
		return EPGSearchResult{}, fmt.Errorf("search EPG: %w", err)
	}

	channelIDs := make(map[string]struct{}, len(criteria.ChannelIDs))
	for _, channelID := range criteria.ChannelIDs {
		channelIDs[strings.TrimSpace(channelID)] = struct{}{}
	}

	filtered := matches[:0]
	for _, event := range matches {
		if len(channelIDs) > 0 {
			if _, ok := channelIDs[event.ChannelID]; !ok {
				continue
			}
		}
		if !epgEventOverlapsWindow(event, criteria.StartsAt, criteria.EndsAt) {
			continue
		}
		filtered = append(filtered, event)
	}

	result := EPGSearchResult{Events: filtered, Total: len(filtered)}
	limit := criteria.ResultLimit
	if limit == 0 {
		limit = defaultEPGSearchLimit
	}
	if len(result.Events) > limit {
		result.Events = result.Events[:limit]
		result.Truncated = true
	}
	return result, nil
}

func validateEPGSearchCriteria(criteria EPGSearchCriteria) error {
	if strings.TrimSpace(criteria.Pattern) == "" {
		return fmt.Errorf("search pattern is required")
	}
	mode := strings.ToLower(strings.TrimSpace(criteria.Mode))
	if mode != "" && mode != "phrase" && mode != "regex" {
		return fmt.Errorf("invalid search mode: %q", criteria.Mode)
	}
	if criteria.StartsAt != nil && criteria.EndsAt != nil && criteria.StartsAt.After(*criteria.EndsAt) {
		return fmt.Errorf("search start time must not be after end time")
	}
	if criteria.ResultLimit < 0 || criteria.ResultLimit > maxEPGSearchLimit {
		return fmt.Errorf("result limit must be between 0 and %d", maxEPGSearchLimit)
	}
	return nil
}

func epgEventOverlapsWindow(event domain.EPGEvent, startsAt, endsAt *time.Time) bool {
	if startsAt != nil && !event.Stop.IsZero() && !event.Stop.After(*startsAt) {
		return false
	}
	if endsAt != nil && !event.Start.IsZero() && !event.Start.Before(*endsAt) {
		return false
	}
	return true
}
