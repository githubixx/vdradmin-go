package services

import (
	"context"
	"fmt"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// TimerService handles timer-related operations
type TimerService struct {
	vdrClient ports.VDRClient
}

// NewTimerService creates a new timer service
func NewTimerService(vdrClient ports.VDRClient) *TimerService {
	return &TimerService{
		vdrClient: vdrClient,
	}
}

// GetAllTimers retrieves all timers
func (s *TimerService) GetAllTimers(ctx context.Context) ([]domain.Timer, error) {
	return s.vdrClient.GetTimers(ctx)
}

// CreateTimer creates a new timer
func (s *TimerService) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := s.validateTimer(timer); err != nil {
		return fmt.Errorf("invalid timer: %w", err)
	}

	return s.vdrClient.CreateTimer(ctx, timer)
}

// CreateTimerFromEPG creates a timer from an EPG event
func (s *TimerService) CreateTimerFromEPG(ctx context.Context, event domain.EPGEvent, priority, lifetime, marginStart, marginEnd int) error {
	start := event.Start.Add(-time.Duration(marginStart) * time.Minute)
	stop := event.Stop.Add(time.Duration(marginEnd) * time.Minute)
	// VDR's timer day spec is effectively the date of the timer start time.
	// If margins push the timer start into the previous day (e.g. events at 00:00),
	// we must adjust the day accordingly, otherwise VDR interprets it as 23:58 on the event day.
	startLocal := start.In(time.Local)
	day := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, time.Local)

	timer := &domain.Timer{
		Active:    true,
		ChannelID: event.ChannelID,
		Day:       day,
		Start:     start,
		Stop:      stop,
		Priority:  priority,
		Lifetime:  lifetime,
		Title:     event.Title,
		EventID:   event.EventID,
	}

	return s.CreateTimer(ctx, timer)
}

// UpdateTimer updates an existing timer
func (s *TimerService) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := s.validateTimer(timer); err != nil {
		return fmt.Errorf("invalid timer: %w", err)
	}

	return s.vdrClient.UpdateTimer(ctx, timer)
}

// DeleteTimer deletes a timer
func (s *TimerService) DeleteTimer(ctx context.Context, timerID int) error {
	if timerID <= 0 {
		return domain.ErrInvalidInput
	}

	return s.vdrClient.DeleteTimer(ctx, timerID)
}

// ToggleTimer toggles a timer's active state
func (s *TimerService) ToggleTimer(ctx context.Context, timerID int) error {
	timers, err := s.vdrClient.GetTimers(ctx)
	if err != nil {
		return err
	}

	for _, timer := range timers {
		if timer.ID == timerID {
			timer.Active = !timer.Active
			return s.vdrClient.UpdateTimer(ctx, &timer)
		}
	}

	return domain.ErrNotFound
}

// CheckConflicts checks for timer conflicts
func (s *TimerService) CheckConflicts(ctx context.Context, newTimer *domain.Timer) ([]domain.Timer, error) {
	timers, err := s.vdrClient.GetTimers(ctx)
	if err != nil {
		return nil, err
	}

	var conflicts []domain.Timer
	for _, timer := range timers {
		if timer.ID == newTimer.ID {
			continue
		}

		// Check if timers overlap
		if timer.ChannelID == newTimer.ChannelID &&
			timersOverlap(&timer, newTimer) {
			conflicts = append(conflicts, timer)
		}
	}

	return conflicts, nil
}

func (s *TimerService) validateTimer(timer *domain.Timer) error {
	if timer == nil {
		return domain.ErrInvalidInput
	}

	if timer.ChannelID == "" {
		return fmt.Errorf("%w: channel ID required", domain.ErrInvalidInput)
	}

	if timer.Stop.Before(timer.Start) {
		return fmt.Errorf("%w: stop time must be after start time", domain.ErrInvalidInput)
	}

	if timer.Priority < 0 || timer.Priority > 99 {
		return fmt.Errorf("%w: priority must be between 0 and 99", domain.ErrInvalidInput)
	}

	if timer.Lifetime < 0 || timer.Lifetime > 99 {
		return fmt.Errorf("%w: lifetime must be between 0 and 99", domain.ErrInvalidInput)
	}

	return nil
}

func timersOverlap(t1, t2 *domain.Timer) bool {
	return t1.Start.Before(t2.Stop) && t2.Start.Before(t1.Stop)
}
