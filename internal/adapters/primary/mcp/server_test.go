package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
	modelcontextprotocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServer_SearchEPG(t *testing.T) {
	start := time.Date(2026, time.August, 27, 18, 0, 0, 0, time.UTC)
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{{ID: "one", Number: 1, Name: "One"}}).
		WithEPGEvents([]domain.EPGEvent{{
			EventID: 1, ChannelID: "one", ChannelNumber: 1, ChannelName: "One", Title: "Science Fiction", Start: start, Stop: start.Add(time.Hour),
		}})
	epgService := services.NewEPGService(mock, time.Minute)
	recordingService := services.NewRecordingService(mock, 0)
	timerService := services.NewTimerService(mock)
	server := NewServer(epgService, recordingService, timerService, "test")
	serverTransport, clientTransport := modelcontextprotocol.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := modelcontextprotocol.NewClient(&modelcontextprotocol.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 4 {
		t.Fatalf("unexpected tools: %+v", tools.Tools)
	}
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		seen[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("expected read-only annotations for %s: %+v", tool.Name, tool.Annotations)
		}
	}
	if !seen[searchEPGToolName] || !seen[searchRecordingsToolName] || !seen[listTimersToolName] || !seen[currentProgramsToolName] {
		t.Fatalf("unexpected tools: %+v", tools.Tools)
	}

	result, err := session.CallTool(ctx, &modelcontextprotocol.CallToolParams{
		Name:      searchEPGToolName,
		Arguments: map[string]any{"pattern": "science", "inTitle": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result was an error: %+v", result.Content)
	}
	if result.StructuredContent == nil {
		t.Fatal("expected structured tool output")
	}
}

func TestServer_GetCurrentPrograms(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, stop := now.Add(-time.Hour), now.Add(time.Hour)
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{
			{ID: "one", Number: 1, Name: "One"},
			{ID: "two", Number: 2, Name: "Two"},
			{ID: "hidden", Number: 3, Name: "Hidden"},
		}).
		WithEPGEvents([]domain.EPGEvent{
			{EventID: 2, ChannelID: "two", Title: "Second", Start: start, Stop: stop},
			{EventID: 1, ChannelID: "one", Title: "First", Subtitle: "Episode", Description: "Details", Start: start, Stop: stop},
			{EventID: 3, ChannelID: "hidden", Title: "Hidden", Start: start, Stop: stop},
			{EventID: 4, ChannelID: "one", Title: "Earlier", Start: start.Add(-time.Hour), Stop: stop},
			{EventID: 5, ChannelID: "one", Title: "Past", Start: start.Add(-time.Hour), Stop: start},
			{EventID: 6, ChannelID: "two", Title: "Future", Start: stop, Stop: stop.Add(time.Hour)},
		})
	epg := services.NewEPGService(mock, 0)
	epg.SetWantedChannels([]string{"one", "two"})
	session := connectTestSession(t, NewServer(epg, services.NewRecordingService(mock, 0), services.NewTimerService(mock), "test"))

	call := func(args map[string]any) currentProgramsOutput {
		t.Helper()
		result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: currentProgramsToolName, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("CallTool: result=%+v err=%v", result, err)
		}
		return decodeStructuredContent[currentProgramsOutput](t, result.StructuredContent)
	}

	output := call(map[string]any{"resultLimit": 1})
	if output.Total != 2 || !output.Truncated || len(output.Events) != 1 {
		t.Fatalf("unexpected result metadata: %+v", output)
	}
	event := output.Events[0]
	if event.EventID != 1 || event.ChannelID != "one" || event.ChannelNumber != 1 || event.ChannelName != "One" ||
		event.Title != "First" || event.Subtitle != "Episode" || event.Description != "Details" ||
		event.StartsAt != start.Format(time.RFC3339) || event.EndsAt != stop.Format(time.RFC3339) || event.DurationSecs != 7200 {
		t.Fatalf("unexpected event: %+v", event)
	}

	filtered := call(map[string]any{"channelIds": []string{" two ", "hidden"}})
	if filtered.Total != 1 || filtered.Truncated || len(filtered.Events) != 1 || filtered.Events[0].EventID != 2 {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}
	empty := call(map[string]any{"channelIds": []string{"missing"}})
	if empty.Total != 0 || empty.Truncated || empty.Events == nil || len(empty.Events) != 0 {
		t.Fatalf("unexpected empty result: %+v", empty)
	}

	for _, limit := range []int{-1, 201} {
		result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{
			Name: currentProgramsToolName, Arguments: map[string]any{"resultLimit": limit},
		})
		if err != nil || !result.IsError {
			t.Fatalf("expected tool error for limit %d: result=%+v err=%v", limit, result, err)
		}
	}
}

func TestServer_GetCurrentProgramsDefaultLimitAndBackendError(t *testing.T) {
	now := time.Now()
	events := make([]domain.EPGEvent, 51)
	channels := make([]domain.Channel, len(events))
	for i := range events {
		id := fmt.Sprintf("channel-%d", i)
		channels[i] = domain.Channel{ID: id, Number: i + 1}
		events[i] = domain.EPGEvent{ChannelID: id, Start: now.Add(-time.Hour), Stop: now.Add(time.Hour)}
	}
	mock := ports.NewMockVDRClient().WithChannels(channels).WithEPGEvents(events)
	session := connectTestSession(t, NewServer(services.NewEPGService(mock, 0), services.NewRecordingService(mock, 0), services.NewTimerService(mock), "test"))
	result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: currentProgramsToolName, Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("CallTool: result=%+v err=%v", result, err)
	}
	output := decodeStructuredContent[currentProgramsOutput](t, result.StructuredContent)
	if output.Total != 51 || !output.Truncated || len(output.Events) != 50 {
		t.Fatalf("unexpected default limit result: %+v", output)
	}

	failing := ports.NewMockVDRClient()
	failing.GetEPGFunc = func(context.Context, string, time.Time) ([]domain.EPGEvent, error) {
		return nil, errors.New("VDR unavailable")
	}
	failedSession := connectTestSession(t, NewServer(services.NewEPGService(failing, 0), services.NewRecordingService(failing, 0), services.NewTimerService(failing), "test"))
	result, err = failedSession.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: currentProgramsToolName, Arguments: map[string]any{}})
	if err != nil || !result.IsError {
		t.Fatalf("expected backend tool error: result=%+v err=%v", result, err)
	}
}

func TestServer_SearchRecordings(t *testing.T) {
	baseDate := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	mock := ports.NewMockVDRClient().
		WithChannels([]domain.Channel{{ID: "one", Number: 1, Name: "One"}}).
		WithRecordings([]domain.Recording{
			{Path: "1", Title: "Planet Earth", Subtitle: "Mountains", Description: "Wildlife documentary", Channel: "BBC", Date: baseDate.Add(-time.Hour), Length: 45 * time.Minute, Size: 1024},
			{Path: "2", Title: "Evening News", Subtitle: "Planet policy", Description: "Daily headlines", Channel: "ARD", Date: baseDate, Length: 30 * time.Minute, Size: 2048},
			{Path: "archive/space-special", Title: "Space Night", Subtitle: "Orbit", Description: "Quiet images", Channel: "BR", Date: baseDate.Add(-2 * time.Hour), Length: 60 * time.Minute, Size: 4096},
			{Path: "4", Title: "Cooking Show", Subtitle: "Pasta", Description: "Weeknight cooking", Channel: "Kitchen", Date: baseDate.Add(-3 * time.Hour), Length: 25 * time.Minute, Size: 512},
		})
	epgService := services.NewEPGService(mock, 0)
	recordingService := services.NewRecordingService(mock, 0)
	timerService := services.NewTimerService(mock)
	server := NewServer(epgService, recordingService, timerService, "test")
	session := connectTestSession(t, server)

	result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{
		Name: searchRecordingsToolName,
		Arguments: map[string]any{
			"pattern":     "planet",
			"inSubtitle":  true,
			"sortBy":      "name",
			"resultLimit": 1,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result was an error: %+v", result.Content)
	}
	output := decodeStructuredContent[searchRecordingsOutput](t, result.StructuredContent)
	if output.Total != 2 || !output.Truncated || len(output.Recordings) != 1 {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
	if output.Recordings[0].Title != "Evening News" {
		t.Fatalf("expected name-sorted first recording, got %+v", output.Recordings[0])
	}
	if output.Recordings[0].Date != baseDate.Format(time.RFC3339) || output.Recordings[0].LengthSeconds != int64((30*time.Minute)/time.Second) || output.Recordings[0].SizeBytes != 2048 {
		t.Fatalf("unexpected recording fields: %+v", output.Recordings[0])
	}

	for name, args := range map[string]map[string]any{
		"path":        {"pattern": "special", "inPath": true},
		"description": {"pattern": "quiet", "inDescription": true},
		"channel":     {"pattern": "kitchen", "inChannel": true},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: searchRecordingsToolName, Arguments: args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool result was an error: %+v", result.Content)
			}
			output := decodeStructuredContent[searchRecordingsOutput](t, result.StructuredContent)
			if output.Total != 1 || len(output.Recordings) != 1 {
				t.Fatalf("expected one match, got %+v", output)
			}
		})
	}

	for name, args := range map[string]map[string]any{
		"empty":     {"pattern": ""},
		"too short": {"pattern": "pl"},
		"bad limit": {"pattern": "planet", "resultLimit": 201},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: searchRecordingsToolName, Arguments: args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error tool result")
			}
		})
	}
}

func TestServer_ListTimers(t *testing.T) {
	firstStart := time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC)
	secondStart := firstStart.Add(2 * time.Hour)
	mock := ports.NewMockVDRClient().WithTimers([]domain.Timer{
		{
			ID: 3, Active: true, ChannelID: "two", DaySpec: "2026-11-18", Title: "Later show",
			Start: secondStart.Add(time.Hour), Stop: secondStart.Add(2 * time.Hour),
			StartMinutes: 780, StopMinutes: 840, Priority: 50, Lifetime: 99,
		},
		{
			ID: 2, Active: true, ChannelID: "one", DaySpec: "MTWTFSS", Title: "Recurring show",
			Start: secondStart, Stop: secondStart.Add(time.Hour),
			StartMinutes: 720, StopMinutes: 780, Priority: 40, Lifetime: 30, EventID: 42,
		},
		{
			ID: 1, Active: false, ChannelID: "one", DaySpec: "2026-11-18", Title: "Inactive show",
			Start: firstStart, Stop: firstStart.Add(time.Hour),
			StartMinutes: 600, StopMinutes: 660, Priority: 20, Lifetime: 10,
		},
	})
	server := NewServer(
		services.NewEPGService(mock, 0),
		services.NewRecordingService(mock, 0),
		services.NewTimerService(mock),
		"test",
	)
	session := connectTestSession(t, server)

	unfilteredResult, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{
		Name:      listTimersToolName,
		Arguments: map[string]any{"resultLimit": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if unfilteredResult.IsError {
		t.Fatalf("tool result was an error: %+v", unfilteredResult.Content)
	}
	unfilteredOutput := decodeStructuredContent[listTimersOutput](t, unfilteredResult.StructuredContent)
	if unfilteredOutput.Total != 3 || !unfilteredOutput.Truncated || len(unfilteredOutput.Timers) != 2 {
		t.Fatalf("unexpected unfiltered result metadata: %+v", unfilteredOutput)
	}
	if unfilteredOutput.Timers[0].ID != 1 || unfilteredOutput.Timers[1].ID != 2 {
		t.Fatalf("expected timers ordered by start time, got %+v", unfilteredOutput.Timers)
	}

	result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{
		Name: listTimersToolName,
		Arguments: map[string]any{
			"active":      true,
			"channelId":   "one",
			"startsAt":    secondStart.Add(-30 * time.Minute).Format(time.RFC3339),
			"endsAt":      secondStart.Add(30 * time.Minute).Format(time.RFC3339),
			"resultLimit": 1,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool result was an error: %+v", result.Content)
	}
	output := decodeStructuredContent[listTimersOutput](t, result.StructuredContent)
	if output.Total != 1 || output.Truncated || len(output.Timers) != 1 {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
	timer := output.Timers[0]
	if timer.ID != 2 || !timer.Active || timer.ChannelID != "one" || timer.Title != "Recurring show" {
		t.Fatalf("unexpected timer fields: %+v", timer)
	}
	if timer.DaySpec != "MTWTFSS" || timer.StartsAt != secondStart.Format(time.RFC3339) || timer.EndsAt != secondStart.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("unexpected timer schedule: %+v", timer)
	}
	if timer.StartMinutes != 720 || timer.StopMinutes != 780 || timer.Priority != 40 || timer.Lifetime != 30 || timer.EventID != 42 {
		t.Fatalf("unexpected timer metadata: %+v", timer)
	}

	for name, args := range map[string]map[string]any{
		"invalid start": {"startsAt": "not-a-time"},
		"reversed range": {
			"startsAt": secondStart.Format(time.RFC3339),
			"endsAt":   firstStart.Format(time.RFC3339),
		},
		"invalid limit": {"resultLimit": 201},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &modelcontextprotocol.CallToolParams{Name: listTimersToolName, Arguments: args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error tool result")
			}
		})
	}
}

func connectTestSession(t *testing.T, server *modelcontextprotocol.Server) *modelcontextprotocol.ClientSession {
	t.Helper()
	serverTransport, clientTransport := modelcontextprotocol.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := modelcontextprotocol.NewClient(&modelcontextprotocol.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func decodeStructuredContent[T any](t *testing.T, content any) T {
	t.Helper()
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("Marshal structured content: %v", err)
	}
	var output T
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("Unmarshal structured content: %v", err)
	}
	return output
}
