package services

import (
	"context"
	"fmt"
	"sort"
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

// GetSchedule returns EPG events overlapping the optional time window, ordered by start time.
func (s *EPGService) GetSchedule(ctx context.Context, channelIDs []string, startsAt, endsAt *time.Time) ([]domain.EPGEvent, error) {
	if startsAt != nil && endsAt != nil && startsAt.After(*endsAt) {
		return nil, fmt.Errorf("endsAt must not be before startsAt")
	}
	events, err := s.GetEPG(ctx, "", time.Time{})
	if err != nil {
		return nil, err
	}

	selected := make(map[string]struct{}, len(channelIDs))
	for _, id := range channelIDs {
		selected[strings.TrimSpace(id)] = struct{}{}
	}
	matches := make([]domain.EPGEvent, 0)
	for _, event := range events {
		if len(selected) > 0 {
			if _, ok := selected[event.ChannelID]; !ok {
				continue
			}
		}
		if epgEventOverlapsWindow(event, startsAt, endsAt) {
			matches = append(matches, event)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if !matches[i].Start.Equal(matches[j].Start) {
			return matches[i].Start.Before(matches[j].Start)
		}
		if matches[i].ChannelID != matches[j].ChannelID {
			return matches[i].ChannelID < matches[j].ChannelID
		}
		return matches[i].EventID < matches[j].EventID
	})
	return matches, nil
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
