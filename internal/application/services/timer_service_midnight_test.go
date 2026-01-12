package services

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

type timerCreateSpyVDR struct {
	created *domain.Timer
}

func (s *timerCreateSpyVDR) Connect(ctx context.Context) error { return nil }
func (s *timerCreateSpyVDR) Close() error                      { return nil }
func (s *timerCreateSpyVDR) Ping(ctx context.Context) error    { return nil }

func (s *timerCreateSpyVDR) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (s *timerCreateSpyVDR) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return nil, nil
}
func (s *timerCreateSpyVDR) GetTimers(ctx context.Context) ([]domain.Timer, error) { return nil, nil }

func (s *timerCreateSpyVDR) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	cpy := *timer
	s.created = &cpy
	return nil
}
func (s *timerCreateSpyVDR) UpdateTimer(ctx context.Context, timer *domain.Timer) error { return nil }
func (s *timerCreateSpyVDR) DeleteTimer(ctx context.Context, timerID int) error         { return nil }

func (s *timerCreateSpyVDR) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return nil, nil
}

func (s *timerCreateSpyVDR) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}
func (s *timerCreateSpyVDR) DeleteRecording(ctx context.Context, path string) error { return nil }
func (s *timerCreateSpyVDR) GetCurrentChannel(ctx context.Context) (string, error)  { return "", nil }
func (s *timerCreateSpyVDR) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (s *timerCreateSpyVDR) SendKey(ctx context.Context, key string) error { return nil }

func TestCreateTimerFromEPG_MidnightMarginAdjustsDay(t *testing.T) {
	loc := time.Local
	// Event starts at midnight; marginStart pushes timer start into previous day.
	eventStart := time.Date(2026, 1, 7, 0, 0, 0, 0, loc)
	eventStop := time.Date(2026, 1, 7, 1, 30, 0, 0, loc)

	event := domain.EPGEvent{
		EventID:   123,
		ChannelID: "C-1-2-3",
		Title:     "Familie Heinz Becker - Lachgeschichten",
		Start:     eventStart,
		Stop:      eventStop,
	}

	spy := &timerCreateSpyVDR{}
	svc := NewTimerService(spy)

	marginStart := 2
	marginEnd := 10
	if err := svc.CreateTimerFromEPG(context.Background(), event, 50, 99, marginStart, marginEnd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.created == nil {
		t.Fatalf("expected CreateTimer to be called")
	}

	expectedStart := eventStart.Add(-2 * time.Minute)
	expectedDay := time.Date(expectedStart.Year(), expectedStart.Month(), expectedStart.Day(), 0, 0, 0, 0, loc)

	if !spy.created.Start.Equal(expectedStart) {
		t.Fatalf("expected timer start %s, got %s", expectedStart.Format(time.RFC3339), spy.created.Start.Format(time.RFC3339))
	}
	if !spy.created.Day.Equal(expectedDay) {
		t.Fatalf("expected timer day %s, got %s", expectedDay.Format("2006-01-02"), spy.created.Day.Format("2006-01-02"))
	}
	if !spy.created.Stop.Equal(eventStop.Add(10 * time.Minute)) {
		t.Fatalf("expected timer stop %s, got %s", eventStop.Add(10*time.Minute).Format(time.RFC3339), spy.created.Stop.Format(time.RFC3339))
	}
}
