package services

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// RecordingService handles recording-related operations
type RecordingService struct {
	vdrClient   ports.VDRClient
	cache       []domain.Recording
	cacheMu     sync.RWMutex
	cacheExpiry time.Duration
	cacheTime   time.Time
}

// NewRecordingService creates a new recording service
func NewRecordingService(vdrClient ports.VDRClient, cacheExpiry time.Duration) *RecordingService {
	return &RecordingService{
		vdrClient:   vdrClient,
		cacheExpiry: cacheExpiry,
	}
}

// GetAllRecordings retrieves all recordings with caching
func (s *RecordingService) GetAllRecordings(ctx context.Context) ([]domain.Recording, error) {
	// If caching is disabled, always fetch fresh data.
	if s.cacheExpiry <= 0 {
		return s.vdrClient.GetRecordings(ctx)
	}

	// Check cache
	s.cacheMu.RLock()
	if time.Now().Before(s.cacheTime.Add(s.cacheExpiry)) && len(s.cache) > 0 {
		recordings := make([]domain.Recording, len(s.cache))
		copy(recordings, s.cache)
		s.cacheMu.RUnlock()
		return recordings, nil
	}
	s.cacheMu.RUnlock()

	// Fetch from VDR
	recordings, err := s.vdrClient.GetRecordings(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.cacheMu.Lock()
	s.cache = recordings
	s.cacheTime = time.Now()
	s.cacheMu.Unlock()

	return recordings, nil
}

// GetRecordingsByFolder retrieves recordings organized as a folder tree
func (s *RecordingService) GetRecordingsByFolder(ctx context.Context) (*domain.Recording, error) {
	recordings, err := s.GetAllRecordings(ctx)
	if err != nil {
		return nil, err
	}

	// Build folder tree
	root := &domain.Recording{
		Path:     "/",
		Title:    "Recordings",
		IsFolder: true,
		Children: make([]*domain.Recording, 0),
	}

	// Simple folder organization - can be enhanced
	for i := range recordings {
		root.Children = append(root.Children, &recordings[i])
	}

	return root, nil
}

// DeleteRecording deletes a recording and invalidates cache
func (s *RecordingService) DeleteRecording(ctx context.Context, path string) error {
	if path == "" {
		return domain.ErrInvalidInput
	}

	if err := s.vdrClient.DeleteRecording(ctx, path); err != nil {
		return err
	}

	// Invalidate cache
	s.InvalidateCache()

	return nil
}

// SortRecordings sorts recordings by various criteria
func (s *RecordingService) SortRecordings(recordings []domain.Recording, sortBy string) []domain.Recording {
	sorted := make([]domain.Recording, len(recordings))
	copy(sorted, recordings)

	switch sortBy {
	case "name":
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].Title != sorted[j].Title {
				return sorted[i].Title < sorted[j].Title
			}
			return sorted[i].Path < sorted[j].Path
		})
	case "date":
		// Newest -> oldest
		sort.SliceStable(sorted, func(i, j int) bool {
			if !sorted[i].Date.Equal(sorted[j].Date) {
				return sorted[i].Date.After(sorted[j].Date)
			}
			return sorted[i].Path < sorted[j].Path
		})
	case "length":
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].Length != sorted[j].Length {
				return sorted[i].Length > sorted[j].Length
			}
			return sorted[i].Path < sorted[j].Path
		})
	default:
		// Default to date (newest -> oldest)
		sort.SliceStable(sorted, func(i, j int) bool {
			if !sorted[i].Date.Equal(sorted[j].Date) {
				return sorted[i].Date.After(sorted[j].Date)
			}
			return sorted[i].Path < sorted[j].Path
		})
	}

	return sorted
}

// InvalidateCache clears the recording cache
func (s *RecordingService) InvalidateCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = nil
	s.cacheTime = time.Time{}
}
