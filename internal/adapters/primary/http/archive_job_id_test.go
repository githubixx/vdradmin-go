package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	nethttp "net/http"

	"github.com/githubixx/vdradmin-go/internal/application/archive"
)

func TestNormalizeArchiveJobID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "whitespace", in: "  \t\n ", want: ""},
		{name: "plain", in: "abc-123", want: "abc-123"},
		{name: "quoted", in: "\"abc-123\"", want: "abc-123"},
		{name: "quoted_with_spaces", in: "  \"abc-123\"  ", want: "abc-123"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeArchiveJobID(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeArchiveJobID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRecordingArchiveJobPoll_UnquotesID(t *testing.T) {
	// Not parallel: this spins a goroutine for the job runner.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := &Handler{
		logger:      logger,
		archiveJobs: archive.NewJobManager(),
		instanceID:  "test-instance",
		pid:         12345,
	}

	outDir := filepath.Join(t.TempDir(), "out")
	plan := archive.Plan{
		Segments: []string{"/does/not/matter/00001.ts"},
		Preview: archive.Preview{
			TargetDir:   outDir,
			VideoPath:   filepath.Join(outDir, "video.mkv"),
			InfoDstPath: filepath.Join(outDir, "video.info"),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ensure ffmpeg is never started; job still gets registered

	jobID, err := h.archiveJobs.Start(ctx, plan, h.instanceID)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	quoted := "\"" + jobID + "\""
	req := httptest.NewRequest(
		nethttp.MethodGet,
		"/recordings/archive/job/poll?id="+url.QueryEscape(quoted)+"&from=0",
		nil,
	)
	w := httptest.NewRecorder()

	h.RecordingArchiveJobPoll(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("poll status=%d, body=%s", resp.StatusCode, string(b))
	}

	var payload struct {
		ID       string   `json:"id"`
		Status   string   `json:"status"`
		Error    string   `json:"error"`
		Instance string   `json:"instance_id"`
		PID      int      `json:"pid"`
		LogNext  int      `json:"log_next"`
		LogLines []string `json:"log_lines"`
		Progress any      `json:"progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	if payload.ID != jobID {
		t.Fatalf("payload id=%q, want %q", payload.ID, jobID)
	}
	if payload.Instance != h.instanceID {
		t.Fatalf("payload instance_id=%q, want %q", payload.Instance, h.instanceID)
	}
	if payload.PID != h.pid {
		t.Fatalf("payload pid=%d, want %d", payload.PID, h.pid)
	}
	if payload.Status == "" {
		t.Fatalf("payload status is empty")
	}
}
