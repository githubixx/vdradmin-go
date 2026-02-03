package services

import (
	"context"
	"path/filepath"
	"os"
	"sort"
	"strings"
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

// SetCacheExpiry updates the recordings cache expiry.
// If expiry <= 0, caching is disabled.
func (s *RecordingService) SetCacheExpiry(expiry time.Duration) {
	s.cacheMu.Lock()
	s.cacheExpiry = expiry
	s.cache = nil
	s.cacheTime = time.Time{}
	s.cacheMu.Unlock()
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
	// Check cache expiry under lock
	s.cacheMu.RLock()
	cacheExpiry := s.cacheExpiry
	s.cacheMu.RUnlock()

	// If caching is disabled, always fetch fresh data.
	if cacheExpiry <= 0 {
		return s.vdrClient.GetRecordings(ctx)
	}

	// Check cache
	s.cacheMu.RLock()
	if time.Now().Before(s.cacheTime.Add(cacheExpiry)) && len(s.cache) > 0 {
		recordings := make([]domain.Recording, len(s.cache))
		copy(recordings, s.cache)
		s.cacheMu.RUnlock()

		// If recordings are removed out-of-band (e.g. deleted on disk), the cached list
		// can still contain entries. If we know the on-disk directory, prune missing ones
		// without forcing a full backend refresh.
		pruned := recordings[:0]
		removed := false
		for _, rec := range recordings {
			diskPath := strings.TrimSpace(rec.DiskPath)
			if diskPath == "" {
				pruned = append(pruned, rec)
				continue
			}
			if _, err := os.Stat(diskPath); err != nil {
				if os.IsNotExist(err) {
					removed = true
					continue
				}
			}
			// VDR recordings should have an `info` file. When a recording is removed
			// manually, the directory may temporarily remain while core files are gone.
			// Treat missing `info` as a strong signal the recording is no longer valid.
			if _, err := os.Stat(filepath.Join(diskPath, "info")); err != nil {
				if os.IsNotExist(err) {
					removed = true
					continue
				}
			}
			pruned = append(pruned, rec)
		}
		if removed {
			// Update cache so subsequent calls stay consistent.
			s.cacheMu.Lock()
			s.cache = append([]domain.Recording(nil), pruned...)
			s.cacheMu.Unlock()
		}
		return pruned, nil
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
	case "date_oldest":
		// Oldest -> newest
		sort.SliceStable(sorted, func(i, j int) bool {
			if !sorted[i].Date.Equal(sorted[j].Date) {
				return sorted[i].Date.Before(sorted[j].Date)
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
