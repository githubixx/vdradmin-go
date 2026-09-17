// Package mcp exposes vdradmin-go use cases through the Model Context Protocol.
package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	searchEPGToolName        = "search_epg"
	searchRecordingsToolName = "search_recordings"
	defaultResultLimit       = 50
	maxResultLimit           = 200
	minRecordingPatternRunes = 3
)

type searchEPGInput struct {
	Pattern     string   `json:"pattern" jsonschema:"The text or regular expression to search for."`
	Mode        string   `json:"mode,omitempty" jsonschema:"Search mode: phrase (default) or regex."`
	MatchCase   bool     `json:"matchCase,omitempty" jsonschema:"Match case when true; phrase searches are case-insensitive by default."`
	InTitle     bool     `json:"inTitle,omitempty" jsonschema:"Search program titles."`
	InSubtitle  bool     `json:"inSubtitle,omitempty" jsonschema:"Search program subtitles."`
	InDesc      bool     `json:"inDescription,omitempty" jsonschema:"Search program descriptions."`
	ChannelIDs  []string `json:"channelIds,omitempty" jsonschema:"Optional VDR channel IDs to include."`
	StartsAt    string   `json:"startsAt,omitempty" jsonschema:"Optional RFC3339 lower time bound; programs overlapping it are included."`
	EndsAt      string   `json:"endsAt,omitempty" jsonschema:"Optional RFC3339 upper time bound; programs overlapping it are included."`
	ResultLimit int      `json:"resultLimit,omitempty" jsonschema:"Maximum number of results, from 1 to 200; defaults to 50."`
}

type searchEPGOutput struct {
	Events    []epgEventOutput `json:"events" jsonschema:"The sorted matching EPG events."`
	Total     int              `json:"total" jsonschema:"Number of matches before the result limit."`
	Truncated bool             `json:"truncated" jsonschema:"Whether additional matches were omitted because of the result limit."`
}

type epgEventOutput struct {
	EventID       int    `json:"eventId"`
	ChannelID     string `json:"channelId"`
	ChannelNumber int    `json:"channelNumber"`
	ChannelName   string `json:"channelName"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`
	Description   string `json:"description,omitempty"`
	StartsAt      string `json:"startsAt"`
	EndsAt        string `json:"endsAt"`
	DurationSecs  int64  `json:"durationSeconds"`
}

type searchRecordingsInput struct {
	Pattern       string `json:"pattern" jsonschema:"The phrase to search for; must be at least 3 characters."`
	InSubtitle    bool   `json:"inSubtitle,omitempty" jsonschema:"Search recording subtitles in addition to titles."`
	InPath        bool   `json:"inPath,omitempty" jsonschema:"Search recording paths in addition to titles."`
	InDescription bool   `json:"inDescription,omitempty" jsonschema:"Search recording descriptions in addition to titles."`
	InChannel     bool   `json:"inChannel,omitempty" jsonschema:"Search recording channel names in addition to titles."`
	SortBy        string `json:"sortBy,omitempty" jsonschema:"Sort order: date (default), name, date_oldest, or length."`
	ResultLimit   int    `json:"resultLimit,omitempty" jsonschema:"Maximum number of results, from 1 to 200; defaults to 50."`
}

type searchRecordingsOutput struct {
	Recordings []recordingOutput `json:"recordings" jsonschema:"The sorted matching recordings."`
	Total      int               `json:"total" jsonschema:"Number of matches before the result limit."`
	Truncated  bool              `json:"truncated" jsonschema:"Whether additional matches were omitted because of the result limit."`
}

type recordingOutput struct {
	Path          string `json:"path"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`
	Description   string `json:"description,omitempty"`
	Channel       string `json:"channel,omitempty"`
	Date          string `json:"date,omitempty"`
	LengthSeconds int64  `json:"lengthSeconds"`
	SizeBytes     int64  `json:"sizeBytes"`
	IsFolder      bool   `json:"isFolder"`
}

// NewServer constructs the MCP server and registers all vdradmin-go tools.
func NewServer(epgService *services.EPGService, recordingService *services.RecordingService, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "vdradmin-go-mcp", Version: version}, &mcp.ServerOptions{
		Instructions: "Search the configured VDR electronic program guide and recordings. Use search_epg for read-only TV show and programme searches, and search_recordings for read-only completed recording searches.",
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        searchEPGToolName,
		Title:       "Search TV programs",
		Description: "Search VDR EPG programs by phrase or regular expression, with optional fields, channels, time window, and result limit.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
		},
	}, searchEPGHandler(epgService))
	mcp.AddTool(server, &mcp.Tool{
		Name:        searchRecordingsToolName,
		Title:       "Search recordings",
		Description: "Search completed VDR recordings by phrase, with optional fields, sort order, and result limit.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
		},
	}, searchRecordingsHandler(recordingService))
	return server
}

func searchEPGHandler(epgService *services.EPGService) mcp.ToolHandlerFor[searchEPGInput, searchEPGOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input searchEPGInput) (*mcp.CallToolResult, searchEPGOutput, error) {
		criteria, err := searchCriteriaFromInput(input)
		if err != nil {
			return nil, searchEPGOutput{}, err
		}
		result, err := epgService.SearchEPGWithCriteria(ctx, criteria)
		if err != nil {
			return nil, searchEPGOutput{}, err
		}

		output := searchEPGOutput{
			Events:    make([]epgEventOutput, 0, len(result.Events)),
			Total:     result.Total,
			Truncated: result.Truncated,
		}
		for _, event := range result.Events {
			duration := event.Duration
			if duration == 0 && !event.Start.IsZero() && !event.Stop.IsZero() {
				duration = event.Stop.Sub(event.Start)
			}
			output.Events = append(output.Events, epgEventOutput{
				EventID:       event.EventID,
				ChannelID:     event.ChannelID,
				ChannelNumber: event.ChannelNumber,
				ChannelName:   event.ChannelName,
				Title:         event.Title,
				Subtitle:      event.Subtitle,
				Description:   event.Description,
				StartsAt:      formatEPGTime(event.Start),
				EndsAt:        formatEPGTime(event.Stop),
				DurationSecs:  int64(duration / time.Second),
			})
		}
		return nil, output, nil
	}
}

func searchCriteriaFromInput(input searchEPGInput) (services.EPGSearchCriteria, error) {
	startsAt, err := parseRFC3339Time("startsAt", input.StartsAt)
	if err != nil {
		return services.EPGSearchCriteria{}, err
	}
	endsAt, err := parseRFC3339Time("endsAt", input.EndsAt)
	if err != nil {
		return services.EPGSearchCriteria{}, err
	}
	return services.EPGSearchCriteria{
		Pattern:     input.Pattern,
		Mode:        strings.TrimSpace(input.Mode),
		MatchCase:   input.MatchCase,
		InTitle:     input.InTitle,
		InSubtitle:  input.InSubtitle,
		InDesc:      input.InDesc,
		ChannelIDs:  input.ChannelIDs,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		ResultLimit: input.ResultLimit,
	}, nil
}

func searchRecordingsHandler(recordingService *services.RecordingService) mcp.ToolHandlerFor[searchRecordingsInput, searchRecordingsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input searchRecordingsInput) (*mcp.CallToolResult, searchRecordingsOutput, error) {
		pattern := strings.TrimSpace(input.Pattern)
		if pattern == "" {
			return nil, searchRecordingsOutput{}, fmt.Errorf("pattern is required")
		}
		if utf8.RuneCountInString(pattern) < minRecordingPatternRunes {
			return nil, searchRecordingsOutput{}, fmt.Errorf("pattern must be at least %d characters", minRecordingPatternRunes)
		}
		limit, err := normalizedResultLimit(input.ResultLimit)
		if err != nil {
			return nil, searchRecordingsOutput{}, err
		}

		recordings, err := recordingService.GetAllRecordings(ctx)
		if err != nil {
			return nil, searchRecordingsOutput{}, err
		}
		matches := filterMatchingRecordings(recordings, pattern, input)
		matches = recordingService.SortRecordings(matches, strings.TrimSpace(input.SortBy))

		total := len(matches)
		truncated := total > limit
		if truncated {
			matches = matches[:limit]
		}

		output := searchRecordingsOutput{
			Recordings: make([]recordingOutput, 0, len(matches)),
			Total:      total,
			Truncated:  truncated,
		}
		for _, recording := range matches {
			output.Recordings = append(output.Recordings, recordingToOutput(recording))
		}
		return nil, output, nil
	}
}

func normalizedResultLimit(value int) (int, error) {
	if value == 0 {
		return defaultResultLimit, nil
	}
	if value < 1 || value > maxResultLimit {
		return 0, fmt.Errorf("resultLimit must be between 1 and %d", maxResultLimit)
	}
	return value, nil
}

func filterMatchingRecordings(recordings []domain.Recording, pattern string, input searchRecordingsInput) []domain.Recording {
	needle := strings.ToLower(pattern)
	matches := make([]domain.Recording, 0, len(recordings))
	for _, recording := range recordings {
		if recordingMatches(recording, needle, input) {
			matches = append(matches, recording)
		}
	}
	return matches
}

func recordingMatches(recording domain.Recording, needle string, input searchRecordingsInput) bool {
	haystackParts := []string{recording.Title}
	if input.InSubtitle {
		haystackParts = append(haystackParts, recording.Subtitle)
	}
	if input.InPath {
		haystackParts = append(haystackParts, recording.Path)
	}
	if input.InDescription {
		haystackParts = append(haystackParts, recording.Description)
	}
	if input.InChannel {
		haystackParts = append(haystackParts, recording.Channel)
	}
	return strings.Contains(strings.ToLower(strings.Join(haystackParts, "\n")), needle)
}

func recordingToOutput(recording domain.Recording) recordingOutput {
	return recordingOutput{
		Path:          recording.Path,
		Title:         recording.Title,
		Subtitle:      recording.Subtitle,
		Description:   recording.Description,
		Channel:       recording.Channel,
		Date:          formatEPGTime(recording.Date),
		LengthSeconds: int64(recording.Length / time.Second),
		SizeBytes:     recording.Size,
		IsFolder:      recording.IsFolder,
	}
}

func parseRFC3339Time(name, value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: must be RFC3339", name)
	}
	return &parsed, nil
}

func formatEPGTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
