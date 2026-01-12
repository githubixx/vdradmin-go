package http

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

type recordingsVDRMock struct {
	recordings []domain.Recording
}

func (m *recordingsVDRMock) Connect(ctx context.Context) error { return nil }
func (m *recordingsVDRMock) Close() error                      { return nil }
func (m *recordingsVDRMock) Ping(ctx context.Context) error    { return nil }

func (m *recordingsVDRMock) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (m *recordingsVDRMock) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return nil, nil
}
func (m *recordingsVDRMock) GetTimers(ctx context.Context) ([]domain.Timer, error) { return nil, nil }
func (m *recordingsVDRMock) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *recordingsVDRMock) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return nil
}
func (m *recordingsVDRMock) DeleteTimer(ctx context.Context, timerID int) error { return nil }

func (m *recordingsVDRMock) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return m.recordings, nil
}

func (m *recordingsVDRMock) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return "", nil
}
func (m *recordingsVDRMock) DeleteRecording(ctx context.Context, path string) error { return nil }
func (m *recordingsVDRMock) GetCurrentChannel(ctx context.Context) (string, error)  { return "", nil }
func (m *recordingsVDRMock) SetCurrentChannel(ctx context.Context, channelID string) error {
	return nil
}
func (m *recordingsVDRMock) SendKey(ctx context.Context, key string) error { return nil }

func TestRecordings_SearchThresholdAndFiltering(t *testing.T) {
	mock := &recordingsVDRMock{
		recordings: []domain.Recording{
			{Path: "1", Title: "Foo Bar", Subtitle: "Pilot", Channel: "ARD"},
			{Path: "2", Title: "Something Else", Subtitle: "Foo In Subtitle", Channel: "ZDF"},
			{Path: "path-token-zzz", Title: "Another One", Subtitle: "Other", Channel: "3sat"},
		},
	}

	recordingService := services.NewRecordingService(mock, 0)

	parsed := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "recordings.html"),
	))

	h := NewHandler(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		parsed,
		nil,
		nil,
		recordingService,
		nil,
	)
	h.SetUIThemeDefault("light")
	h.SetTemplates(map[string]*template.Template{
		"_nav.html":       parsed,
		"recordings.html": parsed,
	})

	// Below threshold: should not filter (still shows both).
	{
		req := httptest.NewRequest(http.MethodGet, "/recordings?q=fo&sort=date", nil)
		rw := httptest.NewRecorder()
		h.RecordingList(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		body := rw.Body.String()
		if !strings.Contains(body, "Foo Bar") || !strings.Contains(body, "Something Else") || !strings.Contains(body, "Another One") {
			t.Fatalf("expected both recordings to be present below threshold")
		}
	}

	// At/above threshold: should filter (default: title only).
	{
		req := httptest.NewRequest(http.MethodGet, "/recordings?q=foo&sort=date", nil)
		rw := httptest.NewRecorder()
		h.RecordingList(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		body := rw.Body.String()
		if !strings.Contains(body, "Foo Bar") {
			t.Fatalf("expected matching recording to be present")
		}
		if strings.Contains(body, "Something Else") {
			t.Fatalf("did not expect non-matching recording to be present")
		}
		if strings.Contains(body, "Another One") {
			t.Fatalf("did not expect non-matching recording to be present")
		}
		if !strings.Contains(body, "id=\"recording-search\"") || !strings.Contains(body, "value=\"foo\"") {
			t.Fatalf("expected search input to render with current query")
		}
	}

	// When including subtitle: should match recordings with subtitle containing query.
	{
		req := httptest.NewRequest(http.MethodGet, "/recordings?q=foo&in_subtitle=1&sort=date", nil)
		rw := httptest.NewRecorder()
		h.RecordingList(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		body := rw.Body.String()
		if !strings.Contains(body, "Foo Bar") || !strings.Contains(body, "Something Else") {
			t.Fatalf("expected both title-match and subtitle-match to be present")
		}
		if !strings.Contains(body, "name=\"in_subtitle\"") || !strings.Contains(body, "checked") {
			t.Fatalf("expected include-subtitle option to be rendered as checked")
		}
	}

	// When including path: should match recordings by path token.
	{
		req := httptest.NewRequest(http.MethodGet, "/recordings?q=tok&in_path=1&sort=date", nil)
		rw := httptest.NewRecorder()
		h.RecordingList(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rw.Code)
		}
		body := rw.Body.String()
		if !strings.Contains(body, "Another One") {
			t.Fatalf("expected path-match recording to be present")
		}
		if strings.Contains(body, "Foo Bar") {
			t.Fatalf("did not expect other recordings to be present")
		}
	}
}
