package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

func testFixture(t *testing.T) (*fixtureServer, *bytes.Buffer) {
	t.Helper()
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	f, err := newFixture(root, "light", slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
		if strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("unexpected handler errors:\n%s", logs.String())
		}
	})
	return f, &logs
}

func request(f http.Handler, method, path, user, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.SetBasicAuth(user, fixturePassword)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res := httptest.NewRecorder()
	f.ServeHTTP(res, req)
	return res
}

func requireStatus(t *testing.T, res *httptest.ResponseRecorder, want int) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("status = %d, want %d: %s", res.Code, want, res.Body.String())
	}
}

func TestEmbeddedThemesWithoutThemeDirectory(t *testing.T) {
	project, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, directory := range []string{"web", "test/e2e/server"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{"templates", "static"} {
		if err := os.CopyFS(filepath.Join(root, "web", directory), os.DirFS(filepath.Join(project, "web", directory))); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	f, err := newFixture(root, "light", slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
		if strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("unexpected handler errors:\n%s", logs.String())
		}
	})
	if ids := f.themes.GetAvailableThemes(); len(ids) != 37 {
		t.Fatalf("embedded theme count = %d, want 37", len(ids))
	}
	for _, id := range f.themes.GetAvailableThemes() {
		requireStatus(t, request(f, "GET", "/themes/"+id+"/theme.css", "admin", "", ""), http.StatusOK)
	}
	for _, path := range []string{
		"/ui/css/base.css", "/ui/js/theme.js", "/ui/js/menu.js", "/ui/js/select.js",
		"/themes/crt-starfield-1/assets/crt-sky.webp",
		"/static/img/themes/crt-starfield-1/crt-sky.webp",
	} {
		requireStatus(t, request(f, "GET", path, "admin", "", ""), http.StatusOK)
	}
	page := request(f, "GET", "/timers", "admin", "", "")
	requireStatus(t, page, http.StatusOK)
	for _, text := range []string{`data-theme-source="bundled"`, `/ui/css/base.css`, `VDRAdmin-go`, `EPG Search`} {
		if !strings.Contains(page.Body.String(), text) {
			t.Errorf("embedded consumer page missing %q", text)
		}
	}
	override := filepath.Join(root, "web", "themes", "light")
	if err := os.MkdirAll(override, 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"theme.yaml": "name: Custom Light\n",
		"theme.css":  ":root { --primary-color: magenta; }\n",
	} {
		if err := os.WriteFile(filepath.Join(override, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.themes.Reload(); err != nil {
		t.Fatal(err)
	}
	page = request(f, "GET", "/timers", "admin", "", "")
	requireStatus(t, page, http.StatusOK)
	if !strings.Contains(page.Body.String(), `data-theme-source="custom"`) {
		t.Error("same-ID override incorrectly enables bundled compatibility styles")
	}
	f.ui.SetUIThemeDefault("system")
	page = request(f, "GET", "/timers", "admin", "", "")
	requireStatus(t, page, http.StatusOK)
	if !strings.Contains(page.Body.String(), `data-theme-source="system"`) {
		t.Error("system mode incorrectly enables bundled compatibility styles")
	}
}

func TestRealPagesAndAuthentication(t *testing.T) {
	f, _ := testFixture(t)
	metadata := f.metadata()
	urls := metadata["urls"].(map[string]string)
	paths := []string{
		urls["playing"], urls["channels"], urls["timers"], urls["recordings"],
		urls["watch"], urls["watchPopup"], urls["watchFullscreen"], urls["search"],
		metadata["eventURL"].(string), "/timers/new", "/timers/edit?id=1", "/epgsearch",
		"/epgsearch/new", "/epgsearch/edit?id=1", "/configurations",
		"/configurations/archive-profiles", "/configurations/ffmpeg-profiles",
		"/recordings/archive/jobs",
	}
	home := request(f, "GET", "/", "admin", "", "")
	requireStatus(t, home, http.StatusFound)
	if home.Header().Get("Location") != "/timers" {
		t.Errorf("unexpected home destination: %s", home.Header().Get("Location"))
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			res := request(f, http.MethodGet, path, "admin", "", "")
			requireStatus(t, res, http.StatusOK)
			if !strings.Contains(res.Body.String(), `data-theme="light"`) {
				t.Errorf("real template/theme missing from %s", path)
			}
			if strings.Contains(res.Body.String(), "Internal Server Error") {
				t.Error("template execution failed after headers were written")
			}
		})
	}
	requireStatus(t, request(f, "GET", "/timers", "", "", ""), http.StatusUnauthorized)
	requireStatus(t, request(f, "GET", "/timers", "wrong", "", ""), http.StatusUnauthorized)
	requireStatus(t, request(f, "GET", "/timers", "viewer", "", ""), http.StatusOK)
	viewer := request(f, "GET", "/timers", "viewer", "", "")
	if strings.Contains(viewer.Body.String(), ">New Timer<") {
		t.Error("viewer was incorrectly granted admin UI")
	}
	requireStatus(t, request(f, "POST", "/timers/toggle?id=1", "viewer", "", ""), http.StatusForbidden)
	requireStatus(t, request(f, "GET", "/configurations/archive-profiles", "viewer", "", ""), http.StatusForbidden)
	for _, path := range []string{"/static/css/base.css", "/static/js/theme.js", "/themes/light/theme.css"} {
		requireStatus(t, request(f, "GET", path, "admin", "", ""), http.StatusOK)
	}
	for path, text := range map[string]string{
		urls["playing"]:                   "Evening News",
		urls["channels"]:                  "Das Erste HD",
		urls["recordings"]:                "The Blue Planet",
		metadata["eventURL"].(string):     "Evening News",
		"/search?q=Planet":                "The Blue Planet",
		"/epgsearch":                      "Planet",
		"/timers/edit?id=1":               "Active scheduled timer",
		"/configurations/ffmpeg-profiles": "ffmpeg",
	} {
		res := request(f, "GET", path, "admin", "", "")
		if !strings.Contains(strings.ToLower(res.Body.String()), strings.ToLower(text)) {
			t.Errorf("%s missing fixture text %q", path, text)
		}
	}
}

func TestTimerVisualStates(t *testing.T) {
	f, _ := testFixture(t)
	path := f.metadata()["urls"].(map[string]string)["timers"]
	res := request(f, "GET", path, "admin", "", "")
	requireStatus(t, res, http.StatusOK)
	item := regexp.MustCompile(`<div class="timer-item ([^"]*)">[\s\S]*?<h3>([^<]+)</h3>`)
	states := make(map[string]string)
	for _, match := range item.FindAllStringSubmatch(res.Body.String(), -1) {
		states[match[2]] = match[1]
	}
	for title, classes := range map[string][]string{
		"Active scheduled timer":   {"active"},
		"Inactive scheduled timer": {"inactive"},
		"Recording in progress":    {"active", "recording"},
		"Overlapping documentary":  {"active", "collision"},
		"Overlapping science":      {"active", "collision"},
		"Conflicting film":         {"active", "collision", "conflict"},
		"Conflicting news":         {"active", "collision", "conflict"},
		"Conflicting concert":      {"active", "collision", "conflict"},
	} {
		for _, class := range classes {
			if !slices.Contains(strings.Fields(states[title]), class) {
				t.Errorf("%q classes = %q, missing %q", title, states[title], class)
			}
		}
	}
	if strings.Contains(states["Overlapping documentary"], "conflict") {
		t.Error("two transponders should not exceed two tuners")
	}
	if !strings.Contains(res.Body.String(), `class="timeline-block`) {
		t.Error("real timer timeline was not rendered")
	}
}

func TestThemeControlsAndReadyMetadata(t *testing.T) {
	f, _ := testFixture(t)
	res := request(f, "GET", "/__test/ready", "", "", "")
	requireStatus(t, res, http.StatusOK)
	var metadata struct {
		OK          bool                         `json:"ok"`
		Themes      []string                     `json:"themes"`
		Date        string                       `json:"date"`
		EventURL    string                       `json:"eventURL"`
		Credentials map[string]map[string]string `json:"credentials"`
		URLs        map[string]string            `json:"urls"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.OK || metadata.Date != fixtureDate || len(metadata.Themes) < 2 || metadata.EventURL == "" {
		t.Fatalf("incomplete ready metadata: %s", res.Body.String())
	}
	if metadata.Credentials["viewer"]["username"] != "viewer" || metadata.Credentials["admin"]["password"] != fixturePassword {
		t.Fatal("unexpected credential contract")
	}
	for _, id := range append(metadata.Themes, "system") {
		res := request(f, "POST", "/__test/theme", "", "application/json", `{"theme":"`+id+`"}`)
		requireStatus(t, res, http.StatusOK)
		page := request(f, "GET", "/timers", "admin", "", "").Body.String()
		if !strings.Contains(page, `data-theme-default="`+id+`"`) {
			t.Errorf("theme %s did not reach the real handler", id)
		}
		if id != "system" {
			requireStatus(t, request(f, "GET", "/themes/"+id+"/theme.css", "admin", "", ""), http.StatusOK)
		}
	}
	requireStatus(t, request(f, "POST", "/__test/theme", "", "application/x-www-form-urlencoded", "theme=dark"), http.StatusOK)
	before, err := os.ReadFile(f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"theme":"missing"}`, `{"theme":"../light"}`, `{`, `{}`, `{"theme":123}`} {
		requireStatus(t, request(f, "POST", "/__test/theme", "", "application/json", body), http.StatusBadRequest)
	}
	after, err := os.ReadFile(f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("invalid theme request changed fixture config")
	}
	requireStatus(t, request(f, "GET", "/__test/theme", "", "", ""), http.StatusMethodNotAllowed)
	requireStatus(t, request(f, "GET", "/__test/reset", "", "", ""), http.StatusMethodNotAllowed)
	requireStatus(t, request(f, "POST", "/__test/ready", "", "", ""), http.StatusMethodNotAllowed)
}

func TestWatchSnapshotAndReset(t *testing.T) {
	f, _ := testFixture(t)
	snapshot := request(f, "GET", "/watch/snapshot?size=quarter", "admin", "", "")
	requireStatus(t, snapshot, http.StatusOK)
	img, err := jpeg.Decode(bytes.NewReader(snapshot.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 480 || img.Bounds().Dy() != 270 {
		t.Fatalf("wrong real handler snapshot dimensions: %v", img.Bounds())
	}
	again := request(f, "GET", "/watch/snapshot?size=quarter", "admin", "", "")
	if !bytes.Equal(snapshot.Body.Bytes(), again.Body.Bytes()) {
		t.Error("same channel snapshot is not deterministic")
	}
	requireStatus(t, request(f, "POST", "/watch/channel", "admin", "application/x-www-form-urlencoded", "channel=2"), http.StatusNoContent)
	switched := request(f, "GET", "/watch/snapshot?size=quarter", "admin", "", "")
	if bytes.Equal(snapshot.Body.Bytes(), switched.Body.Bytes()) {
		t.Error("channel switch did not reach fake snapshot capability")
	}
	requireStatus(t, request(f, "POST", "/watch/key", "admin", "application/x-www-form-urlencoded", "key=VolumePlus"), http.StatusNoContent)
	if len(f.vdr.keys) != 1 || f.vdr.keys[0] != "Volume+" {
		t.Fatalf("real key normalization missing: %v", f.vdr.keys)
	}
	requireStatus(t, request(f, "POST", "/__test/watch", "", "application/json", `{"state":"error"}`), http.StatusOK)
	failed := request(f, "GET", "/watch/snapshot", "admin", "", "")
	requireStatus(t, failed, http.StatusBadGateway)
	if failed.Header().Get("X-VDRAdmin-Error") != "grab failed" {
		t.Error("real WatchTV error handling was not exercised")
	}
	requireStatus(t, request(f, "POST", "/__test/watch", "", "application/json", `{"state":"missing"}`), http.StatusBadRequest)
	// Warm both service caches before mutating their backend state.
	requireStatus(t, request(f, "GET", "/recordings", "admin", "", ""), http.StatusOK)
	requireStatus(t, request(f, "GET", "/playing?day="+fixtureDate, "admin", "", ""), http.StatusOK)
	requireStatus(t, request(f, "POST", "/timers/toggle?id=1", "admin", "", ""), http.StatusOK)
	requireStatus(t, request(f, "DELETE", "/recordings?path="+url.QueryEscape("Evening News"), "admin", "", ""), http.StatusOK)
	requireStatus(t, request(f, "POST", "/__test/theme", "", "application/json", `{"theme":"dark"}`), http.StatusOK)
	requireStatus(t, request(f, "POST", "/__test/reset", "", "", ""), http.StatusOK)
	if f.cfg.UI.Theme != "light" || !f.vdr.timers[0].Active || len(f.vdr.recordings) != 3 || len(f.vdr.keys) != 0 {
		t.Error("reset did not restore complete backend/config state")
	}
	restored := request(f, "GET", "/watch/snapshot?size=quarter", "admin", "", "")
	requireStatus(t, restored, http.StatusOK)
	if !bytes.Equal(snapshot.Body.Bytes(), restored.Body.Bytes()) {
		t.Error("reset did not restore initial channel/snapshot state")
	}
	recordings := request(f, "GET", "/recordings", "admin", "", "")
	if !strings.Contains(recordings.Body.String(), "Evening News") {
		t.Error("recording cache was not invalidated on reset")
	}
	requireStatus(t, request(f, "POST", "/recordings/archive/start", "admin", "", ""), http.StatusNotImplemented)
}

func TestWatchCurrentProgrammeIsStableAndHasWorkingDetails(t *testing.T) {
	f, _ := testFixture(t)
	// Warm the unrestricted event cache before WatchTV asks for its summary.
	requireStatus(t, request(f, "GET", "/event?channel=S19.2E-1-100-1&id=101", "admin", "", ""), http.StatusOK)
	var first []byte
	for _, channel := range f.vdr.channels {
		for range 2 {
			res := request(f, "GET", "/watch/now?channel="+channel.ID, "admin", "", "")
			requireStatus(t, res, http.StatusOK)
			var programme struct {
				Title       string    `json:"title"`
				Start       time.Time `json:"start"`
				Stop        time.Time `json:"stop"`
				MoreInfoURL string    `json:"more_info_url"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &programme); err != nil {
				t.Fatal(err)
			}
			if programme.Title != liveProgrammeTitle || !programme.Start.Equal(f.vdr.today) || !programme.Stop.Equal(f.vdr.today.AddDate(0, 0, 1)) {
				t.Fatalf("unexpected current programme: %s", res.Body.String())
			}
			if programme.MoreInfoURL == "" {
				t.Fatalf("real WatchTV handler did not supply details: %s", res.Body.String())
			}
			details := request(f, "GET", programme.MoreInfoURL, "admin", "", "")
			requireStatus(t, details, http.StatusOK)
			if !strings.Contains(details.Body.String(), liveProgrammeTitle) {
				t.Error("current programme details do not resolve")
			}
			if channel.Number == 1 {
				if first != nil && !bytes.Equal(first, res.Body.Bytes()) {
					t.Error("repeated now responses changed")
				}
				first = bytes.Clone(res.Body.Bytes())
			}
		}
	}
	ctx := context.WithValue(context.Background(), currentProgrammeKey{}, true)
	for hour := range 24 {
		at := f.vdr.today.Add(time.Duration(hour)*time.Hour + 59*time.Minute + 59*time.Second)
		events, err := f.vdr.GetEPG(ctx, "", time.Time{})
		if err != nil || len(events) != len(f.vdr.channels) {
			t.Fatalf("hour %d: current candidates = %d, error %v", hour, len(events), err)
		}
		for _, event := range events {
			if event.Title != liveProgrammeTitle || event.Start.After(at) || !event.Stop.After(at) {
				t.Errorf("hour %d: current summary can change to fixed EPG: %+v", hour, event)
			}
		}
	}
	// Explicit fixed-date lists must not acquire the synthetic live rows.
	fixed := request(f, "GET", f.metadata()["urls"].(map[string]string)["playing"], "admin", "", "")
	if strings.Contains(fixed.Body.String(), liveProgrammeTitle) {
		t.Error("current programme leaked into pinned EPG rows")
	}
}

func TestConcurrentRealHTTPAndThemeControls(t *testing.T) {
	f, _ := testFixture(t)
	server := httptest.NewServer(f)
	defer server.Close()
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			for range 3 {
				for _, path := range []string{"/__test/ready", "/__test/theme", "/timers", "/recordings"} {
					method, body := "GET", ""
					if path == "/__test/theme" {
						method, body = "POST", `{"theme":"dark"}`
					}
					req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
					if err != nil {
						t.Error(err)
						return
					}
					req.SetBasicAuth("admin", fixturePassword)
					req.Header.Set("Content-Type", "application/json")
					res, err := server.Client().Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					_, readErr := io.Copy(io.Discard, res.Body)
					res.Body.Close()
					if res.StatusCode != http.StatusOK || readErr != nil {
						t.Errorf("%s: status %d, read error %v", path, res.StatusCode, readErr)
					}
				}
			}
		})
	}
	wg.Wait()
}

func TestFakePortCRUDAndFixedData(t *testing.T) {
	f, _ := testFixture(t)
	ctx := context.Background()
	vdr := f.vdr
	for name, op := range map[string]func(context.Context) error{"connect": vdr.Connect, "ping": vdr.Ping} {
		if err := op(ctx); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	channels, err := vdr.GetChannels(ctx)
	if err != nil || len(channels) != 4 {
		t.Fatalf("channels: %v %v", channels, err)
	}
	channels[0].Name = "mutated"
	original, _ := vdr.GetChannels(ctx)
	if original[0].Name != "Das Erste HD" {
		t.Error("GetChannels leaked mutable backend state")
	}
	fixedMidnight, err := time.ParseInLocation("2006-01-02", fixtureDate, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	events, err := vdr.GetEPG(ctx, original[0].ID, fixedMidnight)
	if err != nil || len(events) != 3 || events[0].Start.Format("2006-01-02") != fixtureDate {
		t.Fatalf("fixed events: %v %v", events, err)
	}
	events[0].Audio[0].Language = "mutated"
	again, _ := vdr.GetEPG(ctx, "1", fixedMidnight)
	if again[0].Audio[0].Language != "deu" {
		t.Error("GetEPG leaked nested mutable backend state")
	}
	later, _ := vdr.GetEPG(ctx, original[0].ID, events[0].Stop)
	if len(later) != 0 {
		t.Error("EPG at filter did not drop events at their shared midnight stop")
	}
	timer := domain.Timer{Title: "Created fixture timer", ChannelID: original[0].ID}
	if err := vdr.CreateTimer(ctx, &timer); err != nil || timer.ID != 9 {
		t.Fatalf("CreateTimer: id %d, %v", timer.ID, err)
	}
	timer.Active = true
	if err := vdr.UpdateTimer(ctx, &timer); err != nil {
		t.Fatal(err)
	}
	timers, _ := vdr.GetTimers(ctx)
	if !timers[len(timers)-1].Active {
		t.Error("updated timer not persisted")
	}
	if err := vdr.DeleteTimer(ctx, timer.ID); err != nil {
		t.Fatal(err)
	}
	if err := vdr.DeleteTimer(ctx, timer.ID); err == nil {
		t.Error("missing timer delete silently succeeded")
	}
	if err := vdr.UpdateTimer(ctx, &timer); err == nil {
		t.Error("missing timer update silently succeeded")
	}
	dir, err := vdr.GetRecordingDir(ctx, "Evening News")
	if err != nil || !strings.HasPrefix(dir, f.storage+string(filepath.Separator)) {
		t.Fatalf("recording path escaped fixture: %q %v", dir, err)
	}
	if _, err := vdr.GetRecordingDir(ctx, "../../etc"); err == nil {
		t.Error("unknown recording path accepted")
	}
	if err := vdr.DeleteRecording(ctx, "missing"); err == nil {
		t.Error("missing recording delete silently succeeded")
	}
	if err := vdr.SetCurrentChannel(ctx, "missing"); err == nil {
		t.Error("unknown channel switch silently succeeded")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := vdr.CreateTimer(cancelled, &timer); err == nil {
		t.Error("canceled mutation silently succeeded")
	}
	if err := vdr.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFixedEPGSurvivesEveryFixtureHour(t *testing.T) {
	f, _ := testFixture(t)
	day, err := time.ParseInLocation("2006-01-02", fixtureDate, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	stop := day.AddDate(0, 0, 1)
	for hour := range 24 {
		now := day.Add(time.Duration(hour)*time.Hour + 59*time.Minute + 59*time.Second)
		events, err := f.vdr.GetEPG(context.Background(), f.vdr.channels[0].ID, now)
		if err != nil || len(events) != 3 {
			t.Fatalf("hour %d: got %d events, error %v", hour, len(events), err)
		}
		seenStarts := map[time.Time]bool{}
		for _, event := range events {
			if !event.Stop.Equal(stop) || !event.Stop.After(now) || event.Duration != event.Stop.Sub(event.Start) {
				t.Errorf("hour %d: event %q would be hidden or has inconsistent duration", hour, event.Title)
			}
			if seenStarts[event.Start] {
				t.Errorf("duplicate start time for %q", event.Title)
			}
			seenStarts[event.Start] = true
		}
	}
}

func TestTimerFixtureStatesStableThroughoutStartupDay(t *testing.T) {
	f, _ := testFixture(t)
	for hour := range 24 {
		now := f.vdr.today.Add(time.Duration(hour)*time.Hour + 59*time.Minute + 59*time.Second)
		var recording []int
		for _, timer := range f.vdr.timers {
			if !timer.Start.After(now) && timer.Stop.After(now) {
				recording = append(recording, timer.ID)
			}
			if timer.ID != 8 && !timer.Start.After(now) {
				t.Errorf("hour %d: timer %q is no longer upcoming", hour, timer.Title)
			}
		}
		if !slices.Equal(recording, []int{8}) {
			t.Errorf("hour %d: recording IDs = %v, want [8]", hour, recording)
		}
	}
}

func TestStartupRejectsUnknownThemeWithoutStorage(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newFixture(root, "nonexistent-theme", slog.Default()); err == nil {
		t.Error("unknown startup theme silently accepted")
	}
}

func TestCloseRemovesOnlyOwnStorage(t *testing.T) {
	first, _ := testFixture(t)
	second, _ := testFixture(t)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.storage); !os.IsNotExist(err) {
		t.Fatalf("fixture storage not removed: %v", err)
	}
	if _, err := os.Stat(second.configPath); err != nil {
		t.Fatalf("another fixture's storage was touched: %v", err)
	}
}

func TestFixtureSIGTERMCleansStorage(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestFixtureProcess$")
	cmd.Env = append(os.Environ(), "VDRADMIN_E2E_FIXTURE_PROCESS=1", "VDRADMIN_E2E_FIXTURE_ADDR=127.0.0.1:0")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	address := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "browser fixture ready") {
				for _, field := range strings.Fields(scanner.Text()) {
					if strings.HasPrefix(field, "url=") {
						address <- strings.TrimPrefix(field, "url=")
						return
					}
				}
			}
		}
		address <- ""
	}()
	var baseURL string
	select {
	case baseURL = <-address:
		if baseURL == "" {
			t.Fatal("fixture exited without becoming ready")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fixture startup timed out")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(baseURL + "/__test/ready")
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Storage struct {
			Prefix string `json:"prefix"`
		} `json:"storagePathNormalization"`
	}
	err = json.NewDecoder(res.Body).Decode(&metadata)
	res.Body.Close()
	if err != nil || metadata.Storage.Prefix == "" {
		t.Fatalf("read fixture storage metadata: %v", err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("fixture SIGTERM exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
		t.Fatal("fixture SIGTERM shutdown timed out")
	}
	if _, err := os.Stat(metadata.Storage.Prefix); !os.IsNotExist(err) {
		t.Fatalf("SIGTERM left fixture storage behind: %v", err)
	}
}

func TestFixtureRejectsNonLoopbackBinding(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestFixtureProcess$")
	cmd.Env = append(os.Environ(), "VDRADMIN_E2E_FIXTURE_PROCESS=1", "VDRADMIN_E2E_FIXTURE_ADDR=0.0.0.0:18080")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "must bind a loopback IP address") {
		t.Fatalf("non-loopback bind accepted: %v, %s", err, output)
	}
}

func TestFixtureProcess(t *testing.T) {
	if os.Getenv("VDRADMIN_E2E_FIXTURE_PROCESS") != "1" {
		t.Skip("subprocess entry point")
	}
	flag.CommandLine = flag.NewFlagSet("fixture-process", flag.ContinueOnError)
	os.Args = []string{"fixture-process", "--addr", os.Getenv("VDRADMIN_E2E_FIXTURE_ADDR"), "--theme", "light"}
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
