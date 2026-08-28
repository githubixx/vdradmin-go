package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/ports"
	modelcontextprotocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServer_SearchEPG(t *testing.T) {
	start := time.Date(2026, time.August, 27, 18, 0, 0, 0, time.UTC)
	epgService := services.NewEPGService(ports.NewMockVDRClient().
		WithChannels([]domain.Channel{{ID: "one", Number: 1, Name: "One"}}).
		WithEPGEvents([]domain.EPGEvent{{
			EventID: 1, ChannelID: "one", ChannelNumber: 1, ChannelName: "One", Title: "Science Fiction", Start: start, Stop: start.Add(time.Hour),
		}}), time.Minute)
	server := NewServer(epgService, "test")
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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != searchEPGToolName || !tools.Tools[0].Annotations.ReadOnlyHint {
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
