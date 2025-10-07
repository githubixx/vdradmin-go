package services

import (
	"context"
	"regexp"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// AutoTimerService handles automatic timer creation based on patterns
type AutoTimerService struct {
	vdrClient    ports.VDRClient
	timerService *TimerService
	epgService   *EPGService
	autoTimers   []domain.AutoTimer
}

// NewAutoTimerService creates a new autotimer service
func NewAutoTimerService(vdrClient ports.VDRClient, timerService *TimerService, epgService *EPGService) *AutoTimerService {
	return &AutoTimerService{
		vdrClient:    vdrClient,
		timerService: timerService,
		epgService:   epgService,
		autoTimers:   make([]domain.AutoTimer, 0),
	}
}

// AddAutoTimer adds a new autotimer
func (s *AutoTimerService) AddAutoTimer(at domain.AutoTimer) error {
	if at.Pattern == "" {
		return domain.ErrInvalidInput
	}

	// Validate regex if enabled
	if at.UseRegex {
		if _, err := regexp.Compile(at.Pattern); err != nil {
			return err
		}
	}

	at.ID = len(s.autoTimers) + 1
	s.autoTimers = append(s.autoTimers, at)

	return nil
}

// GetAutoTimers returns all autotimers
func (s *AutoTimerService) GetAutoTimers() []domain.AutoTimer {
	return s.autoTimers
}

// DeleteAutoTimer removes an autotimer
func (s *AutoTimerService) DeleteAutoTimer(id int) error {
	for i, at := range s.autoTimers {
		if at.ID == id {
			s.autoTimers = append(s.autoTimers[:i], s.autoTimers[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

// ProcessAutoTimers processes all active autotimers and creates timers
func (s *AutoTimerService) ProcessAutoTimers(ctx context.Context) (int, error) {
	// Get all EPG events
	events, err := s.epgService.GetEPG(ctx, "", time.Time{})
	if err != nil {
		return 0, err
	}

	// Get existing timers to avoid duplicates
	existingTimers, err := s.timerService.GetAllTimers(ctx)
	if err != nil {
		return 0, err
	}

	created := 0

	for _, at := range s.autoTimers {
		if !at.Active {
			continue
		}

		matches := s.findMatches(events, at)

		for _, event := range matches {
			// Check if already recorded or scheduled
			if s.alreadyDone(event.EventID, at.Done) {
				continue
			}

			if s.alreadyScheduled(event, existingTimers) {
				continue
			}

			// Create timer
			err := s.timerService.CreateTimerFromEPG(ctx, event, at.Priority, at.Lifetime, at.MarginStart, at.MarginEnd)
			if err == nil {
				created++
				// Mark as done
				at.Done = append(at.Done, event.EventID)
			}
		}
	}

	return created, nil
}

// findMatches finds EPG events matching an autotimer
func (s *AutoTimerService) findMatches(events []domain.EPGEvent, at domain.AutoTimer) []domain.EPGEvent {
	var matches []domain.EPGEvent

	var pattern *regexp.Regexp
	if at.UseRegex {
		pattern, _ = regexp.Compile(at.Pattern)
	}

	for _, event := range events {
		// Check channel filter
		if len(at.ChannelFilter) > 0 && !s.channelMatches(event.ChannelID, at.ChannelFilter) {
			continue
		}

		// Check time window
		if !s.timeMatches(event.Start, at.TimeStart, at.TimeEnd) {
			continue
		}

		// Check day of week
		if len(at.DayOfWeek) > 0 && !s.dayMatches(event.Start.Weekday(), at.DayOfWeek) {
			continue
		}

		// Check pattern
		if !s.patternMatches(event, at.Pattern, at.UseRegex, at.SearchIn, pattern) {
			continue
		}

		matches = append(matches, event)
	}

	return matches
}

func (s *AutoTimerService) channelMatches(channelID string, filter []string) bool {
	for _, ch := range filter {
		if ch == channelID {
			return true
		}
	}
	return false
}

func (s *AutoTimerService) timeMatches(eventTime time.Time, start, end *time.Time) bool {
	if start == nil && end == nil {
		return true
	}

	hour := eventTime.Hour()
	minute := eventTime.Minute()
	eventMinutes := hour*60 + minute

	if start != nil {
		startMinutes := start.Hour()*60 + start.Minute()
		if eventMinutes < startMinutes {
			return false
		}
	}

	if end != nil {
		endMinutes := end.Hour()*60 + end.Minute()
		if eventMinutes > endMinutes {
			return false
		}
	}

	return true
}

func (s *AutoTimerService) dayMatches(weekday time.Weekday, filter []time.Weekday) bool {
	for _, day := range filter {
		if day == weekday {
			return true
		}
	}
	return false
}

func (s *AutoTimerService) patternMatches(event domain.EPGEvent, pattern string, useRegex bool, scope domain.SearchScope, regex *regexp.Regexp) bool {
	texts := []string{}

	switch scope {
	case domain.SearchTitle:
		texts = []string{event.Title}
	case domain.SearchTitleSubtitle:
		texts = []string{event.Title, event.Subtitle}
	case domain.SearchAll:
		texts = []string{event.Title, event.Subtitle, event.Description}
	}

	if useRegex && regex != nil {
		for _, text := range texts {
			if regex.MatchString(text) {
				return true
			}
		}
	} else {
		patternLower := toLower(pattern)
		for _, text := range texts {
			if contains(toLower(text), patternLower) {
				return true
			}
		}
	}

	return false
}

func (s *AutoTimerService) alreadyDone(eventID int, done []int) bool {
	for _, id := range done {
		if id == eventID {
			return true
		}
	}
	return false
}

func (s *AutoTimerService) alreadyScheduled(event domain.EPGEvent, timers []domain.Timer) bool {
	for _, timer := range timers {
		if timer.EventID == event.EventID {
			return true
		}
	}
	return false
}
