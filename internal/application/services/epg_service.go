package services

import (
	"context"
	"sort"
	"strconv"
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

	wantedMu  sync.RWMutex
	wantedSet map[string]struct{}

	currentMu        sync.RWMutex
	currentPrograms  []domain.EPGEvent
	currentExpiresAt time.Time
	currentExpiry    time.Duration

	channelsMu        sync.RWMutex
	channelsCache     []domain.Channel
	channelsExpiresAt time.Time
	channelsExpiry    time.Duration
}

// SetCacheExpiry updates the EPG cache expiry used for GetEPG.
func (s *EPGService) SetCacheExpiry(expiry time.Duration) {
	s.cacheMu.Lock()
	s.cacheExpiry = expiry
	s.cacheMu.Unlock()
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
		wantedSet:   make(map[string]struct{}),
		// Keep "What's On Now?" snappy; refresh frequently but cheaply.
		currentExpiry:  15 * time.Second,
		channelsExpiry: 5 * time.Minute,
	}
}

// SetWantedChannels configures which channels are considered "wanted" globally.
// An empty list means "all channels".
func (s *EPGService) SetWantedChannels(channelIDs []string) {
	set := make(map[string]struct{}, len(channelIDs))
	for _, id := range channelIDs {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}

	s.wantedMu.Lock()
	s.wantedSet = set
	s.wantedMu.Unlock()
}

// GetChannels returns the channels list in channels.conf order (as reported by VDR).
func (s *EPGService) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	chs, err := s.getAllChannelsCached(ctx)
	if err != nil {
		return nil, err
	}
	if !s.wantedEnabled() {
		return chs, nil
	}

	out := make([]domain.Channel, 0, len(chs))
	for _, ch := range chs {
		if s.isWantedChannel(ch.ID) {
			out = append(out, ch)
		}
	}
	return out, nil
}

// GetAllChannels returns channels without applying the wanted-channel filter.
// This is intended for configuration UIs.
func (s *EPGService) GetAllChannels(ctx context.Context) ([]domain.Channel, error) {
	return s.getAllChannelsCached(ctx)
}

func (s *EPGService) getAllChannelsCached(ctx context.Context) ([]domain.Channel, error) {
	now := time.Now()
	s.channelsMu.RLock()
	if now.Before(s.channelsExpiresAt) && s.channelsCache != nil {
		cached := make([]domain.Channel, len(s.channelsCache))
		copy(cached, s.channelsCache)
		s.channelsMu.RUnlock()
		return cached, nil
	}
	s.channelsMu.RUnlock()

	chs, err := s.vdrClient.GetChannels(ctx)
	if err != nil {
		return nil, err
	}

	s.channelsMu.Lock()
	s.channelsCache = chs
	s.channelsExpiresAt = now.Add(s.channelsExpiry)
	s.channelsMu.Unlock()

	return chs, nil
}

// GetEPG retrieves EPG data with caching
func (s *EPGService) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if channelID != "" && !s.isWantedChannel(channelID) {
		return []domain.EPGEvent{}, nil
	}
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

	if s.wantedEnabled() {
		filtered := events[:0]
		for _, ev := range events {
			if ev.ChannelID == "" {
				continue
			}
			if s.isWantedChannel(ev.ChannelID) {
				filtered = append(filtered, ev)
			}
		}
		events = filtered
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
	now := time.Now()

	// Fast path: serve cached summary.
	s.currentMu.RLock()
	if now.Before(s.currentExpiresAt) && s.currentPrograms != nil {
		cached := make([]domain.EPGEvent, len(s.currentPrograms))
		copy(cached, s.currentPrograms)
		s.currentMu.RUnlock()
		return cached, nil
	}
	s.currentMu.RUnlock()

	// Slow path: fetch EPG once and derive the currently-running event per channel.
	// Using one SVDRP request is significantly faster than calling LSTE per channel.
	events, err := s.vdrClient.GetEPG(ctx, "", time.Time{})
	if err != nil {
		return nil, err
	}

	byChannel := make(map[string]domain.EPGEvent)
	for i := range events {
		ev := events[i]
		if ev.ChannelID == "" {
			continue
		}
		if !s.isWantedChannel(ev.ChannelID) {
			continue
		}
		if ev.Start.After(now) || !ev.Stop.After(now) {
			continue
		}
		prev, ok := byChannel[ev.ChannelID]
		if !ok || prev.Start.Before(ev.Start) {
			byChannel[ev.ChannelID] = ev
		}
	}

	currentPrograms := make([]domain.EPGEvent, 0, len(byChannel))
	for _, ev := range byChannel {
		currentPrograms = append(currentPrograms, ev)
	}

	// Ensure channels.conf order: if the EPG payload doesn't include a numeric channel number,
	// map the channel id back to the LSTC order.
	channels, err := s.getAllChannelsCached(ctx)
	if err == nil {
		numByID := make(map[string]int, len(channels))
		nameByID := make(map[string]string, len(channels))
		for _, ch := range channels {
			numByID[ch.ID] = ch.Number
			nameByID[ch.ID] = ch.Name
		}
		for i := range currentPrograms {
			if currentPrograms[i].ChannelNumber == 0 {
				currentPrograms[i].ChannelNumber = numByID[currentPrograms[i].ChannelID]
			}
			if currentPrograms[i].ChannelName == "" {
				currentPrograms[i].ChannelName = nameByID[currentPrograms[i].ChannelID]
			}
		}
	}

	sort.SliceStable(currentPrograms, func(i, j int) bool {
		ni := currentPrograms[i].ChannelNumber
		nj := currentPrograms[j].ChannelNumber
		if ni != 0 && nj != 0 && ni != nj {
			return ni < nj
		}
		// Fallback: try numeric channel id if present
		idI, _ := strconv.Atoi(currentPrograms[i].ChannelID)
		idJ, _ := strconv.Atoi(currentPrograms[j].ChannelID)
		if idI != 0 && idJ != 0 && idI != idJ {
			return idI < idJ
		}
		if currentPrograms[i].ChannelName != currentPrograms[j].ChannelName {
			return currentPrograms[i].ChannelName < currentPrograms[j].ChannelName
		}
		return currentPrograms[i].Start.Before(currentPrograms[j].Start)
	})

	s.currentMu.Lock()
	s.currentPrograms = currentPrograms
	s.currentExpiresAt = time.Now().Add(s.currentExpiry)
	s.currentMu.Unlock()

	return currentPrograms, nil
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

// InvalidateAllCaches clears EPG, channels, and current-program caches.
func (s *EPGService) InvalidateAllCaches() {
	s.InvalidateCache()

	s.channelsMu.Lock()
	s.channelsCache = nil
	s.channelsExpiresAt = time.Time{}
	s.channelsMu.Unlock()

	s.currentMu.Lock()
	s.currentPrograms = nil
	s.currentExpiresAt = time.Time{}
	s.currentMu.Unlock()
}

func (s *EPGService) wantedEnabled() bool {
	s.wantedMu.RLock()
	enabled := len(s.wantedSet) > 0
	s.wantedMu.RUnlock()
	return enabled
}

func (s *EPGService) isWantedChannel(channelID string) bool {
	if channelID == "" {
		return false
	}
	s.wantedMu.RLock()
	if len(s.wantedSet) == 0 {
		s.wantedMu.RUnlock()
		return true
	}
	_, ok := s.wantedSet[channelID]
	s.wantedMu.RUnlock()
	return ok
}

func (s *EPGService) getCacheKey(channelID string, at time.Time) string {
	if at.IsZero() {
		if channelID == "" {
			return "all"
		}
		return channelID + "_all"
	}
	if channelID == "" {
		return "all_" + at.Format("2006010215")
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
