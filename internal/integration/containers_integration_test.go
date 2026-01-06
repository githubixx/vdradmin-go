//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestContainers_TimersTimelineRenders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	repoRoot := mustRepoRoot(t)

	networkName := fmt.Sprintf("vdradmin-go-it-%d", time.Now().UnixNano())

	nw, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           networkName,
			CheckDuplicate: false,
		},
	})
	if err != nil {
		t.Fatalf("network: %v", err)
	}
	t.Cleanup(func() { _ = nw.Remove(ctx) })

	svdrp, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:    filepath.Join(repoRoot, "test/integration/svdrpstub"),
				Dockerfile: "Dockerfile",
			},
			ExposedPorts: []string{"6419/tcp"},
			Networks:     []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"svdrp"},
			},
			WaitingFor: wait.ForListeningPort("6419/tcp").WithStartupTimeout(45 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("svdrp container: %v", err)
	}
	t.Cleanup(func() { _ = svdrp.Terminate(ctx) })

	cfgPath := writeTempConfig(t, repoRoot, `server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 5s
  write_timeout: 5s
  max_header_bytes: 1048576
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

vdr:
  host: "svdrp"
  port: 6419
  timeout: 2s
  dvb_cards: 2
  wanted_channels: []
  video_dir: "/var/lib/video.00"
  config_dir: "/etc/vdr"
  reconnect_delay: 1s

auth:
  enabled: false

cache:
  epg_expiry: 0s
  recording_expiry: 0s

timer:
  default_priority: 50
  default_lifetime: 99
  default_margin_start: 2
  default_margin_end: 10

epg:
  searches: []

ui:
  theme: system
`)

	app, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: func() testcontainers.ContainerRequest {
			req := testcontainers.ContainerRequest{
				ExposedPorts: []string{"8080/tcp"},
				Networks:     []string{networkName},
				Files: []testcontainers.ContainerFile{
					{HostFilePath: cfgPath, ContainerFilePath: "/app/config.yaml", FileMode: 0o644},
				},
				WaitingFor: wait.ForHTTP("/timers").WithPort("8080/tcp").WithStartupTimeout(90 * time.Second),
			}
			if img := strings.TrimSpace(os.Getenv("VDRADMIN_GO_APP_IMAGE")); img != "" {
				req.Image = img
				return req
			}
			req.FromDockerfile = testcontainers.FromDockerfile{Context: repoRoot, Dockerfile: "deployments/Dockerfile"}
			return req
		}(),
		Started: true,
	})
	if err != nil {
		t.Fatalf("app container: %v", err)
	}
	t.Cleanup(func() { _ = app.Terminate(ctx) })

	baseURL := mustBaseURL(t, ctx, app, "8080/tcp")

	body := mustHTTPGet(t, ctx, baseURL+"/timers")

	// Timeline container present.
	mustContain(t, body, "timer-timeline")

	// And at least one block of each severity.
	mustContain(t, body, "timeline-block ok")
	mustContain(t, body, "timeline-block collision")
	mustContain(t, body, "timeline-block critical")
}

func mustHTTPGet(t *testing.T, ctx context.Context, url string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("http status=%d body=%s", resp.StatusCode, string(b))
	}
	return string(b)
}

func mustBaseURL(t *testing.T, ctx context.Context, c testcontainers.Container, port string) string {
	t.Helper()
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	mapped, err := c.MappedPort(ctx, nat.Port(port))
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	return fmt.Sprintf("http://%s:%s", host, mapped.Port())
}

func mustContain(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		snippet := body
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
		t.Fatalf("expected body to contain %q; got prefix: %q", substr, snippet)
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	d := wd
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}

func writeTempConfig(t *testing.T, repoRoot, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}
