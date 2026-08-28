// Package mcp exposes vdradmin-go use cases through the Model Context Protocol.
package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const searchEPGToolName = "search_epg"

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

// NewServer constructs the MCP server and registers all vdradmin-go tools.
func NewServer(epgService *services.EPGService, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "vdradmin-mcp", Version: version}, &mcp.ServerOptions{
		Instructions: "Search the configured VDR electronic program guide. Use search_epg for read-only TV show and programme searches.",
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
