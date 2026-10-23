package http

import (
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func TestPlayingGuideEvents(t *testing.T) {
	day := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return day.Add(time.Duration(minutes) * time.Minute) }
	event := func(id, start, stop int) playingEventView {
		return playingEventView{
			EPGEvent:    domain.EPGEvent{EventID: id, Title: "Programme", Start: at(start), Stop: at(stop)},
			TimerActive: id == 1,
		}
	}
	type expectedSlot struct {
		id, offset, duration, lane, lanes int
	}
	for _, tc := range []struct {
		name     string
		from, to int
		events   []playingEventView
		want     []expectedSlot
	}{
		{
			name: "two clipped programmes", from: 420, to: 480,
			events: []playingEventView{event(1, 390, 450), event(2, 450, 540)},
			want:   []expectedSlot{{1, 0, 30, 0, 1}, {2, 30, 30, 0, 1}},
		},
		{
			name: "three programmes", from: 420, to: 480,
			events: []playingEventView{event(1, 420, 440), event(2, 440, 460), event(3, 460, 480)},
			want:   []expectedSlot{{1, 0, 20, 0, 1}, {2, 20, 20, 0, 1}, {3, 40, 20, 0, 1}},
		},
		{
			name: "one long programme fills window", from: 420, to: 480,
			events: []playingEventView{event(1, 360, 540)},
			want:   []expectedSlot{{1, 0, 60, 0, 1}},
		},
		{
			name: "exclude exact outside boundaries", from: 420, to: 480,
			events: []playingEventView{event(1, 360, 420), event(2, 420, 480), event(3, 480, 540)},
			want:   []expectedSlot{{2, 0, 60, 0, 1}},
		},
		{
			name: "leading middle and trailing gaps", from: 420, to: 480,
			events: []playingEventView{event(1, 430, 450), event(2, 460, 470)},
			want:   []expectedSlot{{0, 0, 10, 0, 1}, {1, 10, 20, 0, 1}, {0, 30, 10, 0, 1}, {2, 40, 10, 0, 1}, {0, 50, 10, 0, 1}},
		},
		{
			name: "empty channel has full gap", from: 420, to: 480,
			want: []expectedSlot{{0, 0, 60, 0, 1}},
		},
		{
			name: "overlapping entries retain separate lanes", from: 420, to: 480,
			events: []playingEventView{event(1, 420, 460), event(2, 440, 480)},
			want:   []expectedSlot{{1, 0, 40, 0, 2}, {2, 20, 40, 1, 2}},
		},
		{
			name: "independent overlap clusters", from: 420, to: 480,
			events: []playingEventView{event(1, 420, 440), event(2, 430, 435), event(3, 440, 480)},
			want:   []expectedSlot{{1, 0, 20, 0, 2}, {2, 10, 5, 1, 2}, {3, 20, 40, 0, 1}},
		},
		{
			name: "overnight window", from: 22 * 60, to: 26 * 60,
			events: []playingEventView{event(1, 23*60, 25*60)},
			want:   []expectedSlot{{0, 0, 60, 0, 1}, {1, 60, 120, 0, 1}, {0, 180, 60, 0, 1}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slots := playingGuideEvents(tc.events, at(tc.from), at(tc.to))
			if len(slots) != len(tc.want) {
				t.Fatalf("got %d slots, want %d: %+v", len(slots), len(tc.want), slots)
			}
			for i, want := range tc.want {
				got := slots[i]
				if got.EventID != want.id || got.GuideOffset != float64(want.offset*60) ||
					got.GuideDuration != float64(want.duration*60) || got.GuideLane != want.lane ||
					got.GuideLanes != want.lanes || got.GuideGap != (want.id == 0) ||
					got.GuideLast != (want.offset+want.duration == tc.to-tc.from) {
					t.Fatalf("slot %d = %+v, want %+v", i, got, want)
				}
				if want.id == 0 {
					continue
				}
				for _, original := range tc.events {
					if original.EventID == got.EventID &&
						(!got.Start.Equal(original.Start) || !got.Stop.Equal(original.Stop) || got.TimerActive != original.TimerActive) {
						t.Fatalf("layout modified programme data: %+v", got)
					}
				}
			}
		})
	}
}
