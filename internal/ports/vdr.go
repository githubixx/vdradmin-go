package ports

import (
	"context"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// VDRClient defines the interface for communicating with VDR via SVDRP
type VDRClient interface {
	// Connect establishes a connection to VDR
	Connect(ctx context.Context) error

	// Close closes the connection to VDR
	Close() error

	// Ping checks if VDR is reachable
	Ping(ctx context.Context) error

	// GetChannels retrieves all channels
	GetChannels(ctx context.Context) ([]domain.Channel, error)

	// GetEPG retrieves EPG data for a channel or all channels
	GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error)

	// GetTimers retrieves all timers
	GetTimers(ctx context.Context) ([]domain.Timer, error)

	// CreateTimer creates a new timer
	CreateTimer(ctx context.Context, timer *domain.Timer) error

	// UpdateTimer updates an existing timer
	UpdateTimer(ctx context.Context, timer *domain.Timer) error

	// DeleteTimer deletes a timer
	DeleteTimer(ctx context.Context, timerID int) error

	// GetRecordings retrieves all recordings
	GetRecordings(ctx context.Context) ([]domain.Recording, error)

	// GetRecordingDir resolves the on-disk directory path for a recording.
	// For SVDRP-backed clients this typically maps to `LSTR <id> path`.
	GetRecordingDir(ctx context.Context, recordingID string) (string, error)

	// DeleteRecording deletes a recording
	DeleteRecording(ctx context.Context, path string) error

	// GetCurrentChannel returns the current channel
	GetCurrentChannel(ctx context.Context) (string, error)

	// SetCurrentChannel switches to a channel
	SetCurrentChannel(ctx context.Context, channelID string) error

	// SendKey sends a remote control key
	SendKey(ctx context.Context, key string) error
}
