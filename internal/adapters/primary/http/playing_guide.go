package http

import (
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// Events arrive in chronological order; layout intervals never change their labels.
func playingGuideEvents(events []playingEventView, from, to time.Time) []playingEventView {
	slots := make([]playingEventView, 0, len(events)+1)
	cursor := from
	add := func(event playingEventView, start, stop time.Time) {
		event.GuideOffset = start.Sub(from).Seconds()
		event.GuideDuration = stop.Sub(start).Seconds()
		event.GuideLanes = 1
		event.GuideLast = stop.Equal(to)
		slots = append(slots, event)
	}
	gap := func(start, stop time.Time) {
		add(playingEventView{
			EPGEvent: domain.EPGEvent{Start: start, Stop: stop},
			GuideGap: true,
		}, start, stop)
	}
	for _, event := range events {
		start, stop := event.Start, event.Stop
		if start.Before(from) {
			start = from
		}
		if stop.After(to) {
			stop = to
		}
		if !stop.After(start) {
			continue
		}
		if start.After(cursor) {
			gap(cursor, start)
		}
		add(event, start, stop)
		if stop.After(cursor) {
			cursor = stop
		}
	}
	if cursor.Before(to) {
		gap(cursor, to)
	}

	// Preserve overlapping EPG entries in parallel lanes rather than hiding a card.
	var cluster []int
	var laneEnds []float64
	var clusterEnd float64
	finishCluster := func() {
		for _, index := range cluster {
			slots[index].GuideLanes = len(laneEnds)
		}
	}
	for i := range slots {
		slot := &slots[i]
		if slot.GuideGap {
			continue
		}
		if slot.GuideOffset >= clusterEnd {
			finishCluster()
			cluster = cluster[:0]
			laneEnds = laneEnds[:0]
		}
		lane := 0
		for lane < len(laneEnds) && laneEnds[lane] > slot.GuideOffset {
			lane++
		}
		stop := slot.GuideOffset + slot.GuideDuration
		if lane == len(laneEnds) {
			laneEnds = append(laneEnds, stop)
		} else {
			laneEnds[lane] = stop
		}
		slot.GuideLane = lane
		cluster = append(cluster, i)
		if stop > clusterEnd {
			clusterEnd = stop
		}
	}
	finishCluster()
	return slots
}
