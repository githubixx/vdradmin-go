package services

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

// ExecuteSavedEPGSearch runs a saved EPG search definition against the provided events.
// It returns matching events sorted by start time then channel number.
func ExecuteSavedEPGSearch(events []domain.EPGEvent, search config.EPGSearch, channelOrder map[string]int) ([]domain.EPGEvent, error) {
	pattern := strings.TrimSpace(search.Pattern)
	if pattern == "" {
		return []domain.EPGEvent{}, nil
	}

	mode := strings.ToLower(strings.TrimSpace(search.Mode))
	if mode == "" {
		mode = "phrase"
	}

	useTitle := search.InTitle
	useSubtitle := search.InSubtitle
	useDesc := search.InDesc
	if !useTitle && !useSubtitle && !useDesc {
		useTitle, useSubtitle, useDesc = true, true, true
	}

	var re *regexp.Regexp
	if mode == "regex" {
		flags := ""
		if !search.MatchCase {
			flags = "(?i)"
		}
		compiled, err := regexp.Compile(flags + pattern)
		if err != nil {
			return nil, err
		}
		re = compiled
	}

	matches := make([]domain.EPGEvent, 0, 64)
	for _, ev := range events {
		if !savedSearchChannelMatches(ev, search, channelOrder) {
			continue
		}
		if !savedSearchTextMatches(ev, pattern, mode, search.MatchCase, useTitle, useSubtitle, useDesc, re) {
			continue
		}
		matches = append(matches, ev)
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if !matches[i].Start.Equal(matches[j].Start) {
			return matches[i].Start.Before(matches[j].Start)
		}
		ni := matches[i].ChannelNumber
		nj := matches[j].ChannelNumber
		if ni != 0 && nj != 0 && ni != nj {
			return ni < nj
		}
		oci := channelOrder[matches[i].ChannelID]
		ocj := channelOrder[matches[j].ChannelID]
		if oci != 0 && ocj != 0 && oci != ocj {
			return oci < ocj
		}
		if matches[i].ChannelName != matches[j].ChannelName {
			return matches[i].ChannelName < matches[j].ChannelName
		}
		if matches[i].Title != matches[j].Title {
			return matches[i].Title < matches[j].Title
		}
		return matches[i].EventID < matches[j].EventID
	})

	return matches, nil
}

func savedSearchTextMatches(ev domain.EPGEvent, pattern, mode string, matchCase bool, inTitle, inSubtitle, inDesc bool, re *regexp.Regexp) bool {
	texts := make([]string, 0, 3)
	if inTitle {
		texts = append(texts, ev.Title)
	}
	if inSubtitle {
		texts = append(texts, ev.Subtitle)
	}
	if inDesc {
		texts = append(texts, ev.Description)
	}

	switch mode {
	case "regex":
		for _, t := range texts {
			if re != nil && re.MatchString(t) {
				return true
			}
		}
		return false
	default: // phrase
		if !matchCase {
			pattern = toLower(pattern)
		}
		for _, t := range texts {
			if !matchCase {
				t = toLower(t)
			}
			if contains(t, pattern) {
				return true
			}
		}
		return false
	}
}

func savedSearchChannelMatches(ev domain.EPGEvent, search config.EPGSearch, order map[string]int) bool {
	use := strings.ToLower(strings.TrimSpace(search.UseChannel))
	if use == "" || use == "no" {
		return true
	}

	switch use {
	case "single":
		return strings.TrimSpace(ev.ChannelID) != "" && ev.ChannelID == strings.TrimSpace(search.ChannelID)
	case "range":
		from := strings.TrimSpace(search.ChannelFrom)
		to := strings.TrimSpace(search.ChannelTo)
		if from == "" || to == "" {
			return true
		}
		oEv := order[ev.ChannelID]
		oFrom := order[from]
		oTo := order[to]
		if oEv == 0 || oFrom == 0 || oTo == 0 {
			return true
		}
		if oFrom > oTo {
			oFrom, oTo = oTo, oFrom
		}
		return oEv >= oFrom && oEv <= oTo
	default:
		return true
	}
}

// parseHHMM parses "HH:MM" in local time for comparisons; returns minutes since midnight.
func parseHHMM(s string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, false
	}
	// Avoid strconv import here; tiny parser.
	toInt := func(x string) (int, bool) {
		if x == "" {
			return 0, false
		}
		v := 0
		for i := 0; i < len(x); i++ {
			if x[i] < '0' || x[i] > '9' {
				return 0, false
			}
			v = v*10 + int(x[i]-'0')
		}
		return v, true
	}
	h, ok := toInt(parts[0])
	if !ok || h < 0 || h > 23 {
		return 0, false
	}
	m, ok := toInt(parts[1])
	if !ok || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func minutesSinceMidnight(t time.Time) int {
	lt := t.In(time.Local)
	return lt.Hour()*60 + lt.Minute()
}
