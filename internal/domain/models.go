package domain

import "time"

// Channel represents a VDR channel
type Channel struct {
	ID       string
	Number   int
	Name     string
	Provider string
	Freq     string
	Source   string
	Group    string
}

// EPGEvent represents an electronic program guide entry
type EPGEvent struct {
	EventID       int
	ChannelID     string
	ChannelNumber int
	ChannelName   string
	Title         string
	Subtitle      string
	Description   string
	Start         time.Time
	Stop          time.Time
	Duration      time.Duration
	VPS           *time.Time
	Video         VideoInfo
	Audio         []AudioInfo
}

// VideoInfo contains video stream information
type VideoInfo struct {
	Format string // e.g., "16:9", "4:3"
	HD     bool
}

// AudioInfo contains audio stream information
type AudioInfo struct {
	Language string
	Channels int // 1=mono, 2=stereo, 6=5.1
}

// Timer represents a recording timer
type Timer struct {
	ID        int
	Active    bool
	ChannelID string
	Day       time.Time
	Start     time.Time
	Stop      time.Time
	Priority  int
	Lifetime  int // days to keep recording
	Title     string
	Aux       string
	EventID   int
}

// Recording represents a completed recording
type Recording struct {
	Path        string
	Title       string
	Subtitle    string
	Description string
	Channel     string
	Date        time.Time
	Length      time.Duration
	Size        int64
	IsFolder    bool
	Children    []*Recording
}

// AutoTimer represents an automatic timer based on search patterns
type AutoTimer struct {
	ID            int
	Pattern       string
	UseRegex      bool
	SearchIn      SearchScope
	ChannelFilter []string
	TimeStart     *time.Time
	TimeEnd       *time.Time
	DayOfWeek     []time.Weekday
	Priority      int
	Lifetime      int
	MarginStart   int // minutes
	MarginEnd     int // minutes
	Active        bool
	Done          []int // event IDs already recorded
}

// SearchScope defines where to search for AutoTimer patterns
type SearchScope int

const (
	SearchTitle SearchScope = iota
	SearchTitleSubtitle
	SearchAll
)
