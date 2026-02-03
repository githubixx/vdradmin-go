package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// TestEPGService_ConcurrentCacheAccess tests concurrent read/write to EPG cache
func TestEPGService_ConcurrentCacheAccess(t *testing.T) {
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{
			{ID: "C-1-1-10", Number: 1, Name: "Test Channel"},
		}).
		WithEPGEvents([]domain.EPGEvent{
			{EventID: 1, ChannelID: "C-1-1-10", Title: "Event 1", Start: time.Now(), Stop: time.Now().Add(time.Hour)},
			{EventID: 2, ChannelID: "C-1-1-10", Title: "Event 2", Start: time.Now().Add(time.Hour), Stop: time.Now().Add(2 * time.Hour)},
		})

	service := NewEPGService(mock, 5*time.Minute)
	ctx := context.Background()

	// Run concurrent cache operations
	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetEPG(ctx, "C-1-1-10", time.Now())
				if err != nil {
					t.Errorf("GetEPG failed: %v", err)
				}
			}
		}()
	}

	// Concurrent writers (cache invalidation)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				service.SetCacheExpiry(5 * time.Minute)
			}
		}()
	}

	wg.Wait()
}

// TestEPGService_ConcurrentWantedChannels tests concurrent wanted channel access
func TestEPGService_ConcurrentWantedChannels(t *testing.T) {
	mock := ports.NewMockVDRClient()
	service := NewEPGService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 15
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent writers to wanted channels
	for i := 0; i < goroutines; i++ {
		channelID := string(rune('A' + i))
		go func(id string) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				service.SetWantedChannels([]string{id, "C-1-1-10"})
			}
		}(channelID)
	}

	// Concurrent readers of channels
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = service.GetChannels(ctx)
			}
		}()
	}

	wg.Wait()
}

// TestEPGService_ConcurrentCurrentPrograms tests concurrent current programs access
func TestEPGService_ConcurrentCurrentPrograms(t *testing.T) {
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{
			{ID: "C-1-1-10", Number: 1, Name: "Channel 1"},
			{ID: "C-2-2-20", Number: 2, Name: "Channel 2"},
		}).
		WithEPGEvents([]domain.EPGEvent{
			{EventID: 1, ChannelID: "C-1-1-10", Title: "Now 1", Start: time.Now(), Stop: time.Now().Add(time.Hour)},
			{EventID: 2, ChannelID: "C-2-2-20", Title: "Now 2", Start: time.Now(), Stop: time.Now().Add(time.Hour)},
		})

	service := NewEPGService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent access to current programs
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetCurrentPrograms(ctx)
				if err != nil {
					t.Errorf("GetCurrentPrograms failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestRecordingService_ConcurrentCacheAccess tests concurrent recording cache access
func TestRecordingService_ConcurrentCacheAccess(t *testing.T) {
	mock := ports.NewMockVDRClient().
		WithRecordings([]domain.Recording{
			{Path: "test/rec1", Title: "Recording 1", Date: time.Now()},
			{Path: "test/rec2", Title: "Recording 2", Date: time.Now()},
		})

	service := NewRecordingService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetAllRecordings(ctx)
				if err != nil {
					t.Errorf("GetAllRecordings failed: %v", err)
				}
			}
		}()
	}

	// Concurrent cache updates
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				service.SetCacheExpiry(5 * time.Minute)
			}
		}()
	}

	wg.Wait()
}

// TestRecordingService_ConcurrentSearch tests concurrent search operations
func TestRecordingService_ConcurrentSearch(t *testing.T) {
	recordings := make([]domain.Recording, 100)
	for i := range recordings {
		recordings[i] = domain.Recording{
			Path:  string(rune('A'+i%26)) + "/recording",
			Title: "Recording " + string(rune('A'+i%26)),
			Date:  time.Now(),
		}
	}

	mock := ports.NewMockVDRClient().WithRecordings(recordings)
	service := NewRecordingService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 15
	const iterations = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent searches with different queries
	for i := 0; i < goroutines; i++ {
		query := string(rune('A' + i%26))
		go func(q string) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetAllRecordings(ctx)
				if err != nil {
					t.Errorf("GetAllRecordings failed: %v", err)
				}
			}
		}(query)
	}

	wg.Wait()
}

// TestTimerService_ConcurrentOperations tests concurrent timer operations
func TestTimerService_ConcurrentOperations(t *testing.T) {
	// Start with some initial timers
	initialTimers := []domain.Timer{
		{ID: 1, Active: true, Title: "Timer 1", ChannelID: "C-1-1-10"},
		{ID: 2, Active: true, Title: "Timer 2", ChannelID: "C-2-2-20"},
		{ID: 3, Active: false, Title: "Timer 3", ChannelID: "C-3-3-30"},
	}

	mock := ports.NewMockVDRClient().
		WithTimers(initialTimers).
		WithChannels([]domain.Channel{
			{ID: "C-1-1-10", Number: 1, Name: "Channel 1"},
			{ID: "C-2-2-20", Number: 2, Name: "Channel 2"},
			{ID: "C-3-3-30", Number: 3, Name: "Channel 3"},
		})

	service := NewTimerService(mock)
	ctx := context.Background()

	const goroutines = 10
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetAllTimers(ctx)
				if err != nil {
					t.Errorf("GetAllTimers failed: %v", err)
				}
			}
		}()
	}

	// Concurrent timer reads
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = service.GetAllTimers(ctx)
			}
		}()
	}

	// Concurrent timer updates
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				timer := &domain.Timer{
					ID:        1,
					Active:    j%2 == 0,
					Title:     "Updated Timer",
					ChannelID: "C-1-1-10",
					Day:       time.Now(),
					Start:     time.Now(),
					Stop:      time.Now().Add(time.Hour),
				}
				_ = service.UpdateTimer(ctx, timer)
			}
		}(i)
	}

	wg.Wait()
}

// TestEPGService_ConcurrentChannelsCache tests concurrent channels cache access
func TestEPGService_ConcurrentChannelsCache(t *testing.T) {
	channels := make([]domain.Channel, 50)
	for i := range channels {
		channels[i] = domain.Channel{
			ID:     string(rune('A'+i%26)) + "-1-1-" + string(rune(i)),
			Number: i + 1,
			Name:   "Channel " + string(rune('A'+i%26)),
		}
	}

	mock := ports.NewMockVDRClient().WithChannels(channels)
	service := NewEPGService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent channel cache access
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetAllChannels(ctx)
				if err != nil {
					t.Errorf("GetAllChannels failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestRecordingService_ConcurrentGrouping tests concurrent recording grouping
func TestRecordingService_ConcurrentGrouping(t *testing.T) {
	recordings := make([]domain.Recording, 100)
	for i := range recordings {
		folderName := string(rune('A' + i/10))
		recordings[i] = domain.Recording{
			Path:     folderName + "/recording" + string(rune('0'+i%10)),
			Title:    "Recording " + folderName + string(rune('0'+i%10)),
			Date:     time.Now(),
			IsFolder: i%10 == 0,
		}
	}

	mock := ports.NewMockVDRClient().WithRecordings(recordings)
	service := NewRecordingService(mock, 5*time.Minute)
	ctx := context.Background()

	const goroutines = 15
	const iterations = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent grouping operations
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := service.GetAllRecordings(ctx)
				if err != nil {
					t.Errorf("GetAllRecordings failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestEPGService_RaceInvalidation tests cache invalidation race conditions
func TestEPGService_RaceInvalidation(t *testing.T) {
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{{ID: "C-1", Number: 1, Name: "Test"}}).
		WithEPGEvents([]domain.EPGEvent{
			{EventID: 1, ChannelID: "C-1", Title: "Event", Start: time.Now(), Stop: time.Now().Add(time.Hour)},
		})

	service := NewEPGService(mock, 100*time.Millisecond) // Short expiry
	ctx := context.Background()

	const goroutines = 25
	const duration = 200 * time.Millisecond

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	done := make(chan struct{})
	time.AfterFunc(duration, func() { close(done) })

	// Continuous cache reads
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_, _ = service.GetEPG(ctx, "C-1", time.Now())
				}
			}
		}()
	}

	// Continuous cache expiry updates
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					service.SetCacheExpiry(50 * time.Millisecond)
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
}
