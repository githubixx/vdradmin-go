package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

const fixtureDate = "2026-10-17"
const liveProgrammeTitle = "Fixture Live Programme"

// Current-programme summaries and unrestricted EPG lookups both use a zero
// timestamp in production. The fixture marks WatchTVNow requests explicitly.
type currentProgrammeKey struct{}

var _ ports.VDRClient = (*fixtureVDR)(nil)

// All mutable backend state belongs to this fake, never to a real VDR.
type fixtureVDR struct {
	mu         sync.Mutex
	videoDir   string
	today      time.Time
	channels   []domain.Channel
	events     []domain.EPGEvent
	liveEvents []domain.EPGEvent
	timers     []domain.Timer
	recordings []domain.Recording
	current    string
	keys       []string
	watchState string
	nextID     int
}

func newFixtureVDR(videoDir string, now time.Time) *fixtureVDR {
	f := &fixtureVDR{
		videoDir: videoDir,
		today:    time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local),
	}
	f.reset()
	return f
}

func (f *fixtureVDR) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels = []domain.Channel{
		{ID: "S19.2E-1-100-1", Number: 1, Name: "Das Erste HD", Provider: "ARD", Source: "S19.2E", Freq: "11494", Group: "Public"},
		{ID: "S19.2E-1-200-2", Number: 2, Name: "ZDF HD", Provider: "ZDFvision", Source: "S19.2E", Freq: "11362", Group: "Public"},
		{ID: "S19.2E-1-300-3", Number: 3, Name: "arte HD", Provider: "ARD", Source: "S19.2E", Freq: "10744", Group: "Culture"},
		{ID: "S19.2E-1-100-4", Number: 4, Name: "ONE HD", Provider: "ARD", Source: "S19.2E", Freq: "11494", Group: "Culture"},
	}
	day, err := time.ParseInLocation("2006-01-02", fixtureDate, time.Local)
	if err != nil {
		panic(err) // This is a source-code constant, not user input.
	}
	f.events = nil
	f.liveEvents = nil
	titles := [][]string{
		{"Evening News", "The Blue Planet", "Late Night Culture"},
		{"Science Today", "Mountain Adventure", "European Journal"},
		{"Art & Architecture", "A Journey Through Europe", "Jazz in Concert"},
		{"City Stories", "The Weekend Film", "Comedy Showcase"},
	}
	for i, ch := range f.channels {
		liveStop := f.today.AddDate(0, 0, 1)
		f.liveEvents = append(f.liveEvents, domain.EPGEvent{
			EventID: 9001 + i, ChannelID: ch.ID, ChannelNumber: ch.Number, ChannelName: ch.Name,
			Title: liveProgrammeTitle, Subtitle: "Deterministic all-day broadcast",
			Description: "Generated current programme for browser tests. No external stream is used.",
			Start:       f.today, Stop: liveStop, Duration: liveStop.Sub(f.today),
		})
		for j, title := range titles[i] {
			start := day.Add(time.Duration(18+2*j) * time.Hour)
			// Channels hides finished entries on today's selected day. Keeping
			// every stop at midnight preserves all rows at any fixture-day hour.
			stop := day.AddDate(0, 0, 1)
			f.events = append(f.events, domain.EPGEvent{
				EventID: (i+1)*100 + j + 1, ChannelID: ch.ID, ChannelNumber: ch.Number, ChannelName: ch.Name,
				Title: title, Subtitle: "A fixture programme",
				Description: "Deterministic browser fixture programme.\nA second paragraph with details, accents: München, and punctuation: & < >.",
				Start:       start, Stop: stop, Duration: stop.Sub(start),
				Video: domain.VideoInfo{Format: "16:9", HD: true},
				Audio: []domain.AudioInfo{{Language: "deu", Channels: 2}},
			})
		}
	}
	// TimerList has no exported clock injection and uses today's collision window.
	// Tomorrow keeps overlap/conflict states stable without faking production time.
	timerDay := f.today.AddDate(0, 0, 1)
	timer := func(id, channel, hour, minute, duration int, title string, active bool) domain.Timer {
		start := timerDay.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
		return domain.Timer{
			ID: id, ChannelID: f.channels[channel].ID, Active: active,
			Day: timerDay, DaySpec: timerDay.Format("2006-01-02"),
			Start: start, Stop: start.Add(time.Duration(duration) * time.Minute),
			StartMinutes: hour*60 + minute, StopMinutes: hour*60 + minute + duration,
			Title: title, Priority: 50, Lifetime: 99,
		}
	}
	f.timers = []domain.Timer{
		timer(1, 0, 16, 0, 60, "Active scheduled timer", true),
		timer(2, 1, 17, 0, 45, "Inactive scheduled timer", false),
		timer(3, 0, 18, 0, 60, "Overlapping documentary", true),
		timer(4, 1, 18, 15, 60, "Overlapping science", true),
		timer(5, 0, 20, 0, 90, "Conflicting film", true),
		timer(6, 1, 20, 15, 90, "Conflicting news", true),
		timer(7, 2, 20, 30, 90, "Conflicting concert", true),
		{
			ID: 8, ChannelID: f.channels[3].ID, Active: true, Day: f.today,
			DaySpec: f.today.Format("2006-01-02"), Start: f.today, Stop: timerDay,
			Title: "Recording in progress", Priority: 50, Lifetime: 99,
		},
	}
	f.recordings = []domain.Recording{
		{Path: "Documentaries~The Blue Planet", Title: "Documentaries~The Blue Planet", Subtitle: "Oceans", Description: "Explore the oceans and their wildlife.", Channel: "Das Erste HD", Date: day.Add(20 * time.Hour), Length: 90 * time.Minute, Size: 4_500_000_000},
		{Path: "Series~City Stories~Episode 1", Title: "Series~City Stories~Episode 1", Subtitle: "A new beginning", Description: "The first episode of City Stories.", Channel: "ONE HD", Date: day.Add(18 * time.Hour), Length: 45 * time.Minute, Size: 2_200_000_000},
		{Path: "Evening News", Title: "Evening News", Subtitle: "Daily bulletin", Description: "News from around the world.", Channel: "ZDF HD", Date: day.Add(19 * time.Hour), Length: 30 * time.Minute, Size: 950_000_000},
	}
	f.current = f.channels[0].ID
	f.keys = nil
	f.watchState = "snapshot"
	f.nextID = 9
}

func (f *fixtureVDR) setGuideScenario(scenario string) error {
	if scenario != "many" && scenario != "empty" && scenario != "overlap" && scenario != "morning" && scenario != "dense" {
		return fmt.Errorf("invalid guide scenario: %q", scenario)
	}
	f.reset()
	f.mu.Lock()
	defer f.mu.Unlock()
	if scenario == "empty" {
		f.events = nil
		return nil
	}
	for i := range f.events {
		f.events[i].Stop = f.events[i].Start.Add(2 * time.Hour)
		f.events[i].Duration = 2 * time.Hour
	}
	source := slices.Clone(f.events[:3])
	for i := len(f.channels); i < 9; i++ {
		ch := domain.Channel{
			ID: fmt.Sprintf("S19.2E-1-900-%d", i+1), Number: i + 1,
			Name: fmt.Sprintf("Guide Channel %d", i+1),
		}
		f.channels = append(f.channels, ch)
		for j, event := range source {
			event.EventID = (i+1)*100 + j + 1
			event.ChannelID, event.ChannelNumber, event.ChannelName = ch.ID, ch.Number, ch.Name
			f.events = append(f.events, event)
		}
	}
	f.events[0].Title = "A long programme title that wraps naturally within a narrow channel column"
	f.events[0].Subtitle = "A longer subtitle with details about this evening's programme"
	day := f.events[0].Start.Add(-18 * time.Hour)
	events := f.events[:0]
	for _, event := range f.events {
		index := event.EventID%100 - 1
		switch event.ChannelNumber {
		case 1:
			if index == 0 {
				event.Stop = day.Add(18*time.Hour + 30*time.Minute)
			} else if index == 1 {
				event.Start = day.Add(18*time.Hour + 30*time.Minute)
			}
		case 2:
			event.Start = day.Add(18*time.Hour + time.Duration(index)*20*time.Minute)
			event.Stop = event.Start.Add(20 * time.Minute)
			if index == 2 {
				event.Stop = day.Add(21 * time.Hour)
			}
		case 3:
			if index != 0 {
				continue
			}
			event.Start, event.Stop = day.Add(17*time.Hour), day.Add(22*time.Hour)
		case 4:
			event.Start = day.Add([]time.Duration{18*time.Hour + 15*time.Minute, 19*time.Hour + 30*time.Minute, 20 * time.Hour}[index])
			event.Stop = day.Add(time.Duration(19+index) * time.Hour)
		case 5:
			continue
		}
		event.Duration = event.Stop.Sub(event.Start)
		events = append(events, event)
	}
	f.events = events
	event := f.events[0]
	f.timers = append(f.timers, domain.Timer{
		ID: f.nextID, ChannelID: event.ChannelID, Active: false, Title: event.Title,
		Day: event.Start, DaySpec: event.Start.Format("2006-01-02"),
		Start: event.Start, Stop: event.Stop,
		StartMinutes: event.Start.Hour()*60 + event.Start.Minute(),
		StopMinutes:  event.Stop.Hour()*60 + event.Stop.Minute(),
	})
	f.nextID++
	if scenario == "overlap" {
		event.EventID = 9901
		event.Title = "Overlapping programme information"
		event.Start = day.Add(18*time.Hour + 10*time.Minute)
		event.Stop = day.Add(18*time.Hour + 40*time.Minute)
		event.Duration = event.Stop.Sub(event.Start)
		f.events = append(f.events, event)
	}
	if scenario == "morning" || scenario == "dense" {
		f.events = nil
		f.timers = nil
		for _, channel := range f.channels {
			if channel.Number == 5 {
				continue
			}
			windows := [][2]int{{7 * 60, 9 * 60}, {9 * 60, 24 * 60}}
			if channel.Number == 3 {
				windows = [][2]int{{5*60 + 30, 9 * 60}, {9 * 60, 10 * 60}, {10 * 60, 24 * 60}}
			} else if channel.Number == 9 {
				windows = [][2]int{{6 * 60, 7*60 + 1}, {7*60 + 1, 9 * 60}, {9 * 60, 24 * 60}}
			}
			if scenario == "dense" {
				windows = [][2]int{{9 * 60, 10 * 60}, {10 * 60, 11 * 60}}
				switch channel.Number {
				case 1:
					windows[0][0] = 8*60 + 45
				case 2:
					windows = [][2]int{{9*60 + 50, 10 * 60}, {10 * 60, 10*60 + 10}, {10*60 + 10, 11 * 60}}
				case 4:
					windows = [][2]int{{9*60 + 5, 10*60 + 30}, {10*60 + 30, 11 * 60}}
				case 9:
					windows = [][2]int{{9*60 + 50, 9*60 + 55}, {9*60 + 55, 10 * 60}, {10 * 60, 11 * 60}}
				}
			}
			for i, window := range windows {
				if scenario == "dense" {
					window[0] -= 50
					window[1] -= 50
				}
				start, stop := day.Add(time.Duration(window[0])*time.Minute), day.Add(time.Duration(window[1])*time.Minute)
				title := fmt.Sprintf("Morning programme %d", i+1)
				subtitle := "A morning fixture programme"
				if scenario == "morning" && channel.Number == 3 && i == 0 {
					title = "ARD-Morgenmagazin"
				}
				if scenario == "dense" {
					subtitle = "Regional stories, landscapes and interviews with detailed background information about today's morning programme"
					if channel.Number == 1 && i == 0 {
						title = "Morning panorama with regional views"
					}
				}
				f.events = append(f.events, domain.EPGEvent{
					EventID:   channel.Number*100 + i + 1,
					ChannelID: channel.ID, ChannelNumber: channel.Number, ChannelName: channel.Name,
					Title: title, Subtitle: subtitle,
					Start: start, Stop: stop, Duration: stop.Sub(start),
				})
			}
		}
	}
	return nil
}

func (f *fixtureVDR) Connect(ctx context.Context) error { return ctx.Err() }
func (f *fixtureVDR) Close() error                      { return nil }
func (f *fixtureVDR) Ping(ctx context.Context) error    { return ctx.Err() }

func (f *fixtureVDR) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.channels), ctx.Err()
}

func (f *fixtureVDR) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	source := f.events
	if at.IsZero() {
		// Unrestricted lookups must also resolve WatchTV's real MoreInfoURL.
		source = append(slices.Clone(source), f.liveEvents...)
	}
	if ctx.Value(currentProgrammeKey{}) == true {
		// Exclude fixed evening broadcasts so they cannot win the production
		// service's "latest matching start" selection during the fixture day.
		source = f.liveEvents
	}
	var events []domain.EPGEvent
	for _, ev := range source {
		if channelID != "" && channelID != ev.ChannelID && channelID != strconv.Itoa(ev.ChannelNumber) {
			continue
		}
		if !at.IsZero() && (ev.Stop.Before(at) || ev.Stop.Equal(at)) {
			continue
		}
		ev.Audio = slices.Clone(ev.Audio)
		events = append(events, ev)
	}
	return events, ctx.Err()
}

func (f *fixtureVDR) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.timers), ctx.Err()
}

func (f *fixtureVDR) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	timer.ID = f.nextID
	f.nextID++
	f.timers = append(f.timers, *timer)
	return nil
}

func (f *fixtureVDR) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.timers {
		if f.timers[i].ID == timer.ID {
			f.timers[i] = *timer
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fixtureVDR) DeleteTimer(ctx context.Context, id int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.timers {
		if f.timers[i].ID == id {
			f.timers = slices.Delete(f.timers, i, i+1)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fixtureVDR) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.recordings), ctx.Err()
}

func (f *fixtureVDR) GetRecordingDir(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, recording := range f.recordings {
		if id == recording.Path || id == strconv.Itoa(i+1) {
			return filepath.Join(f.videoDir, "recording-"+strconv.Itoa(i+1)), nil
		}
	}
	return "", domain.ErrNotFound
}

func (f *fixtureVDR) DeleteRecording(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.recordings {
		if f.recordings[i].Path == path {
			f.recordings = slices.Delete(f.recordings, i, i+1)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fixtureVDR) GetCurrentChannel(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current, ctx.Err()
}

func (f *fixtureVDR) SetCurrentChannel(ctx context.Context, channel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range f.channels {
		if ch.ID == channel || strconv.Itoa(ch.Number) == channel {
			f.current = ch.ID
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fixtureVDR) SendKey(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, key)
	return nil
}

func (f *fixtureVDR) setWatchState(state string) error {
	switch state {
	case "snapshot", "error":
	default:
		return fmt.Errorf("unknown watch state %q (expected snapshot or error)", state)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watchState = state
	return nil
}

// GrabJpeg satisfies the optional capability used by the real WatchTV handler.
func (f *fixtureVDR) GrabJpeg(ctx context.Context, width, height int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	state, current := f.watchState, f.current
	f.mu.Unlock()
	if state == "error" {
		return nil, fmt.Errorf("fixture snapshot unavailable")
	}
	if width < 1 || height < 1 || width > 1920 || height > 1080 {
		return nil, fmt.Errorf("invalid snapshot dimensions %dx%d", width, height)
	}
	palette := []color.RGBA{
		{235, 235, 235, 255}, {210, 190, 40, 255}, {30, 175, 180, 255},
		{45, 170, 70, 255}, {180, 50, 175, 255}, {190, 45, 45, 255}, {40, 55, 160, 255},
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	shift := int(current[len(current)-1]-'1') % len(palette)
	for y := range height {
		for x := range width {
			col := palette[(x*len(palette)/width+shift)%len(palette)]
			if y > height*3/4 {
				col = color.RGBA{24, 32, 48, 255}
			}
			img.SetRGBA(x, y, col)
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
