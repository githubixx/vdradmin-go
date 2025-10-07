package services

import (
	"context"
	"sync"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// EPGService handles EPG-related operations with caching
type EPGService struct {
	vdrClient   ports.VDRClient
	cache       map[string]*epgCache
	cacheMu     sync.RWMutex
	cacheExpiry time.Duration
}

type epgCache struct {
	events    []domain.EPGEvent
	expiresAt time.Time
}

// NewEPGService creates a new EPG service
func NewEPGService(vdrClient ports.VDRClient, cacheExpiry time.Duration) *EPGService {
	return &EPGService{
		vdrClient:   vdrClient,
		cache:       make(map[string]*epgCache),
		cacheExpiry: cacheExpiry,
	}
}

// GetEPG retrieves EPG data with caching
func (s *EPGService) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	cacheKey := s.getCacheKey(channelID, at)

	// Check cache first
	s.cacheMu.RLock()
	if cached, ok := s.cache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
		s.cacheMu.RUnlock()
		return cached.events, nil
	}
	s.cacheMu.RUnlock()

	// Fetch from VDR
	events, err := s.vdrClient.GetEPG(ctx, channelID, at)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.cacheMu.Lock()
	s.cache[cacheKey] = &epgCache{
		events:    events,
		expiresAt: time.Now().Add(s.cacheExpiry),
	}
	s.cacheMu.Unlock()

	return events, nil
}

// GetCurrentPrograms returns what's currently playing on all channels
func (s *EPGService) GetCurrentPrograms(ctx context.Context) ([]domain.EPGEvent, error) {
	return s.GetEPG(ctx, "", time.Now())
}

// SearchEPG searches for programs matching criteria
func (s *EPGService) SearchEPG(ctx context.Context, query string) ([]domain.EPGEvent, error) {
	// Get all EPG data
	events, err := s.GetEPG(ctx, "", time.Time{})
	if err != nil {
		return nil, err
	}

	// Simple search in title and description
	var results []domain.EPGEvent
	queryLower := toLower(query)

	for _, event := range events {
		if contains(toLower(event.Title), queryLower) ||
			contains(toLower(event.Subtitle), queryLower) ||
			contains(toLower(event.Description), queryLower) {
			results = append(results, event)
		}
	}

	return results, nil
}

// InvalidateCache clears the EPG cache
func (s *EPGService) InvalidateCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]*epgCache)
}

func (s *EPGService) getCacheKey(channelID string, at time.Time) string {
	if at.IsZero() {
		return "all"
	}
	return channelID + "_" + at.Format("2006010215")
}

// Helper functions
func toLower(s string) string {
	// Simple ASCII lowercase for now
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result[i] = s[i] + 32
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
