package http

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/githubixx/vdradmin-go/internal/application/archive"
	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
	"github.com/githubixx/vdradmin-go/internal/ports"
)

// Handler handles HTTP requests
type Handler struct {
	logger           *slog.Logger
	templates        *template.Template
	templateMap      map[string]*template.Template
	cfg              *config.Config
	configPath       string
	vdrClient        ports.VDRClient
	archiveJobs      *archive.JobManager
	instanceID       string
	pid              int
	nowFunc          func() time.Time
	epgService       *services.EPGService
	timerService     *services.TimerService
	recordingService *services.RecordingService
	autoTimerService *services.AutoTimerService
	uiThemeDefault   string
	hlsProxy         *HLSProxy
	watchTVChannelMu sync.Mutex
}

func (h *Handler) now() time.Time {
	if h != nil && h.nowFunc != nil {
		return h.nowFunc()
	}
	return time.Now()
}

// NewHandler creates a new HTTP handler
func NewHandler(
	logger *slog.Logger,
	templates *template.Template,
	epgService *services.EPGService,
	timerService *services.TimerService,
	recordingService *services.RecordingService,
	autoTimerService *services.AutoTimerService,
) *Handler {
	return &Handler{
		logger:           logger,
		templates:        templates,
		templateMap:      make(map[string]*template.Template),
		cfg:              nil,
		configPath:       "",
		vdrClient:        nil,
		archiveJobs:      archive.NewJobManager(),
		instanceID:       fmt.Sprintf("%d", time.Now().UnixNano()),
		pid:              os.Getpid(),
		nowFunc:          time.Now,
		epgService:       epgService,
		timerService:     timerService,
		recordingService: recordingService,
		autoTimerService: autoTimerService,
		uiThemeDefault:   "system",
	}
}

// SetConfig wires the runtime configuration pointer and file path.
// The pointer must be the same one used to build the middleware/routes.
func (h *Handler) SetConfig(cfg *config.Config, configPath string) {
	h.cfg = cfg
	h.configPath = configPath

	// Initialize HLS proxy if streamdev backend is configured
	if IsHLSProxyEnabled(cfg.VDR.StreamdevBackendURL) {
		if h.hlsProxy != nil {
			h.hlsProxy.Shutdown()
		}
		proxy, err := NewHLSProxy(h.logger, cfg.VDR.StreamdevBackendURL)
		if err != nil {
			h.logger.Error("failed to initialize HLS proxy", slog.Any("error", err))
		} else {
			h.hlsProxy = proxy
			h.logger.Info("HLS proxy enabled", slog.String("backend", cfg.VDR.StreamdevBackendURL))
		}
	}
}

// SetVDRClient provides the VDR client so we can apply VDR connection changes immediately.
func (h *Handler) SetVDRClient(client ports.VDRClient) {
	h.vdrClient = client
}

// SetUIThemeDefault configures the default theme mode (system/light/dark).
func (h *Handler) SetUIThemeDefault(theme string) {
	h.uiThemeDefault = normalizeTheme(theme)
}

// SetTemplates sets the template map
func (h *Handler) SetTemplates(templates map[string]*template.Template) {
	h.templateMap = templates
}

// Home renders the home page
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	// Treat "/" as an entry point: redirect to the configured landing page.
	// When the landing page is explicitly set to "/", render the "What's on now" page.
	if landing := h.landingPath(); landing != "/" {
		http.Redirect(w, r, landing, http.StatusFound)
		return
	}
	h.WhatsOnNow(w, r)
}

// WhatsOnNow renders the current-program overview.
func (h *Handler) WhatsOnNow(w http.ResponseWriter, r *http.Request) {
	loc := time.Local
	data := map[string]any{}

	selectedPreset, atValue, atTime, atErr := parseWhatsOnAtParam(r, loc)
	data["SelectedHour"] = selectedPreset
	data["SelectedAt"] = atValue
	if atErr != nil {
		data["HomeError"] = atErr.Error()
	}

	// Build hour preset options: "now" + 00..23.
	hourOptions := make([]string, 0, 25)
	hourOptions = append(hourOptions, "now")
	for i := 0; i < 24; i++ {
		hourOptions = append(hourOptions, fmt.Sprintf("%02d", i))
	}
	data["HourOptions"] = hourOptions

	events, err := h.epgService.GetProgramsAt(r.Context(), atTime)
	if err != nil {
		// Keep the UI reachable even if VDR/SVDRP is unavailable.
		h.logger.Error("EPG fetch error on home", slog.Any("error", err))
		if data["HomeError"] == nil {
			data["HomeError"] = err.Error()
		}
		data["Events"] = []domain.EPGEvent{}
	} else {
		data["Events"] = events
	}

	h.renderTemplate(w, r, "index.html", data)
}

func parseWhatsOnAtParam(r *http.Request, loc *time.Location) (selectedPreset string, atValue string, atTime time.Time, err error) {
	if loc == nil {
		loc = time.Local
	}

	selectedPreset = strings.TrimSpace(r.URL.Query().Get("h"))
	if selectedPreset == "" {
		selectedPreset = "now"
	}

	atValue = strings.TrimSpace(r.URL.Query().Get("at"))
	localNow := time.Now().In(loc)

	// If no explicit time is provided, derive a default from the preset.
	if atValue == "" {
		if selectedPreset == "now" {
			atValue = localNow.Format("15:04")
		} else {
			h, convErr := strconv.Atoi(selectedPreset)
			if convErr != nil || h < 0 || h > 23 {
				selectedPreset = "now"
				atValue = localNow.Format("15:04")
				err = fmt.Errorf("invalid hour preset")
			} else {
				atValue = fmt.Sprintf("%02d:00", h)
			}
		}
	}

	// Validate HH:MM strictly.
	parsed, parseErr := time.Parse("15:04", atValue)
	if parseErr != nil {
		// Fall back to now, but surface the error.
		selectedPreset = "now"
		atValue = localNow.Format("15:04")
		parsed, _ = time.Parse("15:04", atValue)
		if err == nil {
			err = fmt.Errorf("invalid time (expected HH:MM)")
		}
	}

	atTime = time.Date(localNow.Year(), localNow.Month(), localNow.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)
	return selectedPreset, atValue, atTime, err
}

func (h *Handler) landingPath() string {
	if h.cfg == nil {
		return "/"
	}
	// Config.Validate() normalizes/validates this, but keep a safe fallback.
	p := strings.TrimSpace(h.cfg.UI.LoginPage)
	if p == "" {
		return "/"
	}
	switch p {
	case "/", "/now", "/channels", "/playing", "/timers", "/recordings", "/search", "/epgsearch", "/configurations":
		return p
	default:
		return "/"
	}
}

func pageNameForPath(path string) string {
	// Normalize common entry points.
	if path == "" || path == "/" || path == "/now" {
		return "What's on now"
	}

	// Treat archive job pages as their own top-level section.
	if strings.HasPrefix(path, "/recordings/archive/jobs") || strings.HasPrefix(path, "/recordings/archive/job") {
		return "Jobs"
	}

	// Group sub-pages under their main navigation entry.
	switch {
	case strings.HasPrefix(path, "/channels"):
		return "Channels"
	case strings.HasPrefix(path, "/playing"):
		return "Playing Today"
	case strings.HasPrefix(path, "/watch"):
		return "Watch TV"
	case strings.HasPrefix(path, "/timers"):
		return "Timers"
	case strings.HasPrefix(path, "/recordings"):
		return "Recordings"
	case strings.HasPrefix(path, "/search"):
		return "Search"
	case strings.HasPrefix(path, "/epgsearch"):
		return "EPG Search"
	case strings.HasPrefix(path, "/configurations"):
		return "Configurations"
	default:
		return ""
	}
}

type vdrPictureGrabber interface {
	GrabJpeg(ctx context.Context, width int, height int) ([]byte, error)
}

func parseWatchTVSize(size string) (width int, height int) {
	const maxWidth = 1920
	const maxHeight = 1080
	// Keep behavior aligned with the classic vdradmin-am TV page.
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "full":
		return maxWidth, maxHeight
	case "half", "":
		return maxWidth / 2, maxHeight / 2
	case "quarter":
		return maxWidth / 4, maxHeight / 4
	default:
		return maxWidth / 4, maxHeight / 4
	}
}

func watchTVCurrentChannelIDFromSVDRPChan(cur string, channels []domain.Channel) string {
	cur = strings.TrimSpace(cur)
	if cur == "" {
		return ""
	}
	// VDR's "CHAN" response is typically "<number> <name>".
	fields := strings.Fields(cur)
	if len(fields) > 0 {
		if n, err := strconv.Atoi(fields[0]); err == nil {
			for _, ch := range channels {
				if ch.Number == n {
					return ch.ID
				}
			}
		}
	}
	// If cur is already a channel ID (or we can't map it), return as-is.
	return cur
}

// WatchTV renders the snapshot-based TV page (SVDRP GRAB + remote control).
func (h *Handler) WatchTV(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}

	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	if interval == "" {
		interval = "5"
	}
	data["Interval"] = interval

	size := strings.TrimSpace(r.URL.Query().Get("size"))
	if size == "" {
		size = "half"
	}
	data["Size"] = size

	data["NewWin"] = strings.TrimSpace(r.URL.Query().Get("new_win")) == "1"
	data["FullTV"] = strings.TrimSpace(r.URL.Query().Get("full_tv")) == "1"

	channels, err := h.epgService.GetChannels(r.Context())
	if err != nil {
		h.logger.Error("channels fetch error", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["Channels"] = []domain.Channel{}
		h.renderTemplate(w, r, "watch.html", data)
		return
	}
	data["Channels"] = channels

	// If HLS proxy is enabled, use internal proxy URL; otherwise use stream_url_template if set
	streamTemplate := ""
	if h.cfg != nil {
		if IsHLSProxyEnabled(h.cfg.VDR.StreamdevBackendURL) && h.hlsProxy != nil {
			// HLS proxy mode: build internal proxy URL with {channel} placeholder
			streamTemplate = "/watch/stream/{channel}/index.m3u8"
			data["StreamHint"] = "This uses the built-in HLS proxy."
		} else {
			streamTemplate = strings.TrimSpace(h.cfg.VDR.StreamURLTemplate)
			if streamTemplate != "" {
				data["StreamHint"] = "This uses a configured stream URL template."
			}
		}
	}
	data["StreamURLTemplate"] = streamTemplate

	cur, err := h.vdrClient.GetCurrentChannel(r.Context())
	if err != nil {
		// Non-fatal; still render UI.
		h.logger.Warn("current channel fetch error", slog.Any("error", err))
	}
	data["CurrentChannel"] = watchTVCurrentChannelIDFromSVDRPChan(cur, channels)

	h.renderTemplate(w, r, "watch.html", data)
}

// WatchTVKey sends a remote control key via SVDRP HITK.
func (h *Handler) WatchTVKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimSpace(r.FormValue("key"))
	keyLower := strings.ToLower(key)
	// Match classic vdradmin-am behavior.
	switch key {
	case "VolumePlus":
		key = "Volume+"
	case "VolumeMinus":
		key = "Volume-"
	}
	// Be liberal in what we accept from the UI.
	switch keyLower {
	case "mute":
		key = "Mute"
	case "pause":
		key = "Pause"
	case "volumeminus", "volume-":
		key = "Volume-"
	case "volumeplus", "volume+":
		key = "Volume+"
	}
	if key == "" {
		http.Error(w, "Missing key", http.StatusBadRequest)
		return
	}

	if err := h.vdrClient.SendKey(r.Context(), key); err != nil {
		h.handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// WatchTVChannel switches to a channel via SVDRP CHAN.
func (h *Handler) WatchTVChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sleepCtx := func(ctx context.Context, d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		}
	}

	// Serialize channel switching to avoid overlapping StopAll/CHAN sequences which can
	// cause tuner contention and intermittent SVDRP 554 responses.
	h.watchTVChannelMu.Lock()
	defer h.watchTVChannelMu.Unlock()
	if r.Context().Err() != nil {
		return
	}

	ch := strings.TrimSpace(r.FormValue("channel"))
	if ch == "" {
		http.Error(w, "Missing channel", http.StatusBadRequest)
		return
	}

	// For stream mode we need the numeric channel number to start the HLS proxy.
	// The value posted in "channel" is the VDR channel id (or a numeric id), which may
	// not be safe to use in the streamdev URL template.
	channelNum := strings.TrimSpace(r.FormValue("channel_num"))
	prevChannelNum := strings.TrimSpace(r.FormValue("prev_channel_num"))

	// If streaming is enabled, stop any active ffmpeg process first to free tuners.
	if h.hlsProxy != nil {
		h.hlsProxy.StopAll()
		// Give VDR/streamdev a moment to release tuner resources.
		if !sleepCtx(r.Context(), 600*time.Millisecond) {
			return
		}
	}

	// VDR can sometimes reject a fast switch with SVDRP 554 (e.g. tuner still busy).
	// Retry a few times with a small backoff to make switching more resilient.
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if r.Context().Err() != nil {
			return
		}
		if err := h.vdrClient.SetCurrentChannel(r.Context(), ch); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "svdrp error 554") {
				break
			}
			// backoff: 250ms, 500ms, 800ms, 1200ms
			switch attempt {
			case 1:
				if !sleepCtx(r.Context(), 250*time.Millisecond) {
					return
				}
			case 2:
				if !sleepCtx(r.Context(), 500*time.Millisecond) {
					return
				}
			case 3:
				if !sleepCtx(r.Context(), 800*time.Millisecond) {
					return
				}
			case 4:
				if !sleepCtx(r.Context(), 1200*time.Millisecond) {
					return
				}
			}
		}
	}
	if r.Context().Err() != nil {
		return
	}
	if lastErr != nil {
		// If we killed the old stream already, try to bring it back so the UI keeps playing.
		if h.hlsProxy != nil && prevChannelNum != "" {
			_ = h.hlsProxy.Start(prevChannelNum)
		}

		msg := lastErr.Error()
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(msg), "svdrp error 554") {
			status = http.StatusConflict
		}
		h.logger.Warn("watch channel switch failed", slog.String("channel", ch), slog.String("error", msg))
		if status == http.StatusConflict {
			w.Header().Set("Retry-After", "1")
		}
		http.Error(w, msg, status)
		return
	}

	// Start the HLS proxy for the newly tuned channel so stale playlist requests from the
	// previous channel can't restart the old ffmpeg process.
	if h.hlsProxy != nil {
		if channelNum != "" {
			if err := h.hlsProxy.Start(channelNum); err != nil {
				h.logger.Error("failed to start HLS stream", slog.String("channel_num", channelNum), slog.Any("error", err))
				http.Error(w, "Failed to start stream", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

type watchTVNowResponse struct {
	ChannelID   string    `json:"channel_id"`
	EventID     int       `json:"event_id"`
	Title       string    `json:"title"`
	Subtitle    string    `json:"subtitle"`
	Description string    `json:"description"`
	Start       time.Time `json:"start"`
	Stop        time.Time `json:"stop"`
	MoreInfoURL string    `json:"more_info_url"`
}

// WatchTVNow returns the currently-running EPG event for a channel.
// The channel is identified by its channel id (same value used by POST /watch/channel).
func (h *Handler) WatchTVNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.epgService == nil {
		http.Error(w, "EPG service not available", http.StatusServiceUnavailable)
		return
	}

	channelID := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channelID == "" {
		http.Error(w, "Missing channel", http.StatusBadRequest)
		return
	}

	events, err := h.epgService.GetCurrentPrograms(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	var cur *domain.EPGEvent
	for i := range events {
		if strings.TrimSpace(events[i].ChannelID) == channelID {
			cur = &events[i]
			break
		}
	}
	if cur == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := watchTVNowResponse{
		ChannelID:   cur.ChannelID,
		EventID:     cur.EventID,
		Title:       cur.Title,
		Subtitle:    cur.Subtitle,
		Description: cur.Description,
		Start:       cur.Start,
		Stop:        cur.Stop,
	}
	if cur.EventID > 0 {
		resp.MoreInfoURL = fmt.Sprintf("/event?channel=%s&id=%d", cur.ChannelID, cur.EventID)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeWatchTVSnapshotError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": msg,
	})
}

func humanizeWatchTVSnapshotError(err error) string {
	if err == nil {
		return "unknown error"
	}

	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "grab image failed") {
		return "Snapshot unavailable: this VDR cannot grab frames on the host (SVDRP GRAB needs a primary video output/decoder device; headless recording-only setups typically cannot support this)."
	}

	return msg
}

// WatchTVSnapshot returns a JPEG snapshot via SVDRP GRAB.
func (h *Handler) WatchTVSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	grabber, ok := h.vdrClient.(vdrPictureGrabber)
	if !ok {
		w.Header().Set("X-VDRAdmin-Error", "VDR client does not support GRAB")
		writeWatchTVSnapshotError(w, http.StatusNotImplemented, "VDR does not support snapshot (SVDRP GRAB)")
		return
	}

	width, height := parseWatchTVSize(r.URL.Query().Get("size"))
	img, err := grabber.GrabJpeg(r.Context(), width, height)
	if err != nil {
		h.logger.Warn("grab snapshot failed", slog.Any("error", err))
		w.Header().Set("X-VDRAdmin-Error", "grab failed")
		writeWatchTVSnapshotError(w, http.StatusBadGateway, humanizeWatchTVSnapshotError(err))
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(img)
}

// WatchTVStreamPlaylist serves HLS playlist for a channel via HLS proxy.
func (h *Handler) WatchTVStreamPlaylist(w http.ResponseWriter, r *http.Request) {
	if h.hlsProxy == nil {
		http.Error(w, "HLS proxy not enabled", http.StatusNotImplemented)
		return
	}

	channelNum := r.PathValue("channel")
	if channelNum == "" {
		http.Error(w, "Missing channel", http.StatusBadRequest)
		return
	}

	h.hlsProxy.GetPlaylist(w, r, channelNum)
}

// WatchTVStreamSegment serves HLS segment for a channel via HLS proxy.
func (h *Handler) WatchTVStreamSegment(w http.ResponseWriter, r *http.Request) {
	if h.hlsProxy == nil {
		http.Error(w, "HLS proxy not enabled", http.StatusNotImplemented)
		return
	}

	channelNum := r.PathValue("channel")
	segmentName := r.PathValue("segment")
	if channelNum == "" || segmentName == "" {
		http.Error(w, "Missing channel or segment", http.StatusBadRequest)
		return
	}

	h.hlsProxy.GetSegment(w, r, channelNum, segmentName)
}

type channelsDayGroup struct {
	Day    time.Time
	Events []channelsEventView
}

type channelsEventView struct {
	domain.EPGEvent
	TimerActive bool
}

type channelsDayOption struct {
	Value string
	Label string
}

type playingEventView struct {
	domain.EPGEvent
	TimerActive bool
}

type playingChannelGroup struct {
	Channel domain.Channel
	Anchor  string
	Events  []playingEventView
}

type playingChannelJumpOption struct {
	Anchor string
	Label  string
}

func parseDayParam(r *http.Request, loc *time.Location) (time.Time, error) {
	dayStr := r.URL.Query().Get("day")
	if dayStr == "" {
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc), nil
	}
	t, err := time.ParseInLocation("2006-01-02", dayStr, loc)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
}

// Channels renders the channel view with a channel selector and per-channel EPG list.
func (h *Handler) Channels(w http.ResponseWriter, r *http.Request) {
	channels, err := h.epgService.GetChannels(r.Context())
	data := map[string]any{}
	if err != nil {
		h.logger.Error("channels fetch error", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["Channels"] = []domain.Channel{}
		h.renderTemplate(w, r, "channels.html", data)
		return
	}
	data["Channels"] = channels

	selected := r.URL.Query().Get("channel")
	if selected == "" && len(channels) > 0 {
		selected = channels[0].ID
	}
	data["SelectedChannel"] = selected

	loc := time.Local
	dayStart, err := parseDayParam(r, loc)
	if err != nil {
		h.logger.Error("invalid day parameter", slog.Any("error", err))
		dayStart = time.Now().In(loc)
		dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, loc)
	}
	dayEnd := dayStart.Add(24 * time.Hour)
	data["SelectedDay"] = dayStart.Format("2006-01-02")

	numberByID := make(map[string]int, len(channels))
	idByNumber := make(map[int]string, len(channels))
	for _, ch := range channels {
		if ch.ID != "" {
			numberByID[ch.ID] = ch.Number
		}
		if ch.Number > 0 && ch.ID != "" {
			idByNumber[ch.Number] = ch.ID
		}
	}

	var events []domain.EPGEvent
	daysByValue := map[string]time.Time{}
	// Always include selected day in the dropdown.
	daysByValue[dayStart.Format("2006-01-02")] = dayStart
	if selected != "" {
		// Anchor the EPG request to the selected day. Using time.Now() here causes
		// inconsistent behavior across day selections (and cache keys).
		ev, err := h.epgService.GetEPG(r.Context(), selected, dayStart)
		if err != nil {
			h.logger.Error("EPG fetch error on channels", slog.Any("error", err), slog.String("channel", selected))
			data["HomeError"] = err.Error()
		} else {
			now := time.Now().In(loc)
			for i := range ev {
				// Build day dropdown from available EPG entries.
				d := time.Date(ev[i].Start.In(loc).Year(), ev[i].Start.In(loc).Month(), ev[i].Start.In(loc).Day(), 0, 0, 0, 0, loc)
				daysByValue[d.Format("2006-01-02")] = d

				// Filter to the selected day.
				if ev[i].Start.Before(dayEnd) && ev[i].Stop.After(dayStart) {
					// Keep today's view compact by hiding already-finished programs.
					if dayStart.Equal(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)) {
						if !ev[i].Stop.After(now) {
							continue
						}
					}
					events = append(events, ev[i])
				}
			}
		}
	}

	// Build timer occurrence index for the selected channel/day and mark events as scheduled
	// only when a timer overlaps *and* matches the event title.
	timersByID := map[int]domain.Timer{}
	occByChannelNumber := map[int][]timerOccurrence{}
	occByChannelID := map[string][]timerOccurrence{}
	selectedChannelNumber := 0
	for _, ch := range channels {
		if ch.ID == selected {
			selectedChannelNumber = ch.Number
			break
		}
	}
	if h.timerService != nil && selected != "" {
		timers, tErr := h.timerService.GetAllTimers(r.Context())
		if tErr != nil {
			h.logger.Warn("timers fetch error for channels", slog.Any("error", tErr))
		} else {
			from := dayStart.Add(-24 * time.Hour)
			to := dayEnd.Add(24 * time.Hour)
			for _, t := range timers {
				if strings.TrimSpace(t.ChannelID) == "" {
					continue
				}
				timersByID[t.ID] = t
				occs := timerOccurrences(t, from, to)
				occByChannelID[t.ChannelID] = append(occByChannelID[t.ChannelID], occs...)
				if selectedChannelNumber > 0 {
					if n, err := strconv.Atoi(strings.TrimSpace(t.ChannelID)); err == nil && n == selectedChannelNumber {
						occByChannelNumber[n] = append(occByChannelNumber[n], occs...)
					}
				}
				if selected != "" && strings.TrimSpace(t.ChannelID) == strings.TrimSpace(selected) {
					if selectedChannelNumber > 0 {
						occByChannelNumber[selectedChannelNumber] = append(occByChannelNumber[selectedChannelNumber], occs...)
					}
				}
			}
		}
	}

	eventScheduled := func(ev domain.EPGEvent) bool {
		_, ok := scheduledTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID)
		return ok
	}

	dayOptions := make([]channelsDayOption, 0, len(daysByValue))
	for _, d := range daysByValue {
		dayOptions = append(dayOptions, channelsDayOption{Value: d.Format("2006-01-02"), Label: d.Format("Mon 2006-01-02")})
	}
	sort.SliceStable(dayOptions, func(i, j int) bool { return dayOptions[i].Value < dayOptions[j].Value })
	data["Days"] = dayOptions

	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		return events[i].EventID < events[j].EventID
	})

	// Single selected-day group.
	if len(events) > 0 {
		views := make([]channelsEventView, 0, len(events))
		for _, e := range events {
			if strings.TrimSpace(e.ChannelID) == "" {
				e.ChannelID = selected
			}
			if e.ChannelNumber <= 0 {
				e.ChannelNumber = selectedChannelNumber
			}
			views = append(views, channelsEventView{EPGEvent: e, TimerActive: eventScheduled(e)})
		}
		data["DayGroups"] = []channelsDayGroup{{Day: dayStart, Events: views}}
	} else {
		data["DayGroups"] = []channelsDayGroup{}
	}

	h.renderTemplate(w, r, "channels.html", data)
}

// Configurations renders a simple configuration page.
// For now it only allows switching between system/light/dark theme.
func (h *Handler) Configurations(w http.ResponseWriter, r *http.Request) {
	data := h.configurationsBaseData(r)
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		data["Message"] = msg
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("err")); errMsg != "" {
		data["Error"] = errMsg
	}
	h.renderTemplate(w, r, "configurations.html", data)
}

func (h *Handler) configurationsBaseData(r *http.Request) map[string]any {
	data := map[string]any{}
	if h.cfg != nil {
		data["Config"] = h.cfg
		profiles := h.archiveProfilesFromConfig(h.cfg)
		data["ArchiveProfiles"] = profiles
		data["ArchiveProfilesDerived"] = len(h.cfg.Archive.Profiles) == 0
		if len(profiles) > 0 {
			data["ArchiveProfileSelectedID"] = profiles[0].ID
			data["ArchiveProfileSelected"] = &profiles[0]
		}
		wanted := make(map[string]bool, len(h.cfg.VDR.WantedChannels))
		for _, id := range h.cfg.VDR.WantedChannels {
			wanted[id] = true
		}
		data["WantedChannelSet"] = wanted
	}
	if h.epgService != nil {
		chs, err := h.epgService.GetAllChannels(r.Context())
		if err == nil {
			data["AllChannels"] = chs
		} else {
			h.logger.Warn("channels fetch error for configurations", slog.Any("error", err))
			data["AllChannels"] = []domain.Channel{}
		}
	}
	return data
}

func (h *Handler) archiveProfilesFromConfig(cfg *config.Config) []archive.ArchiveProfile {
	if cfg == nil {
		return nil
	}
	if len(cfg.Archive.Profiles) == 0 {
		return archive.DefaultProfiles(cfg.Archive.BaseDir)
	}
	out := make([]archive.ArchiveProfile, 0, len(cfg.Archive.Profiles))
	for _, p := range cfg.Archive.Profiles {
		k := archive.KindMovie
		if strings.ToLower(strings.TrimSpace(p.Kind)) == "series" {
			k = archive.KindSeries
		}
		out = append(out, archive.ArchiveProfile{ID: p.ID, Name: p.Name, Kind: k, BaseDir: p.BaseDir})
	}
	return out
}

func (h *Handler) defaultProfileIDForKind(profiles []archive.ArchiveProfile, k archive.Kind) string {
	for _, p := range profiles {
		if p.Kind == k {
			return p.ID
		}
	}
	return ""
}

// ConfigurationsApply applies configuration changes without persisting them.
func (h *Handler) ConfigurationsApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	updated, err := h.buildConfigFromForm(r.PostForm)
	if err != nil {
		data := h.configurationsBaseData(r)
		data["Error"] = err.Error()
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}

	if err := h.applyRuntimeConfig(updated); err != nil {
		data := h.configurationsBaseData(r)
		data["Error"] = err.Error()
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}
	http.Redirect(w, r, "/configurations?msg="+url.QueryEscape("Applied configuration (not saved)."), http.StatusSeeOther)
}

// ConfigurationsSave persists configuration changes to the config file and applies them.
func (h *Handler) ConfigurationsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	updated, err := h.buildConfigFromForm(r.PostForm)
	if err != nil {
		data := h.configurationsBaseData(r)
		data["Error"] = err.Error()
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}

	restartRequired := serverRestartRequired(h.cfg.Server, updated.Server)

	// Persist
	if strings.TrimSpace(h.configPath) == "" {
		data := h.configurationsBaseData(r)
		data["Error"] = "No config path configured; cannot save."
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}
	if err := updated.Save(h.configPath); err != nil {
		data := h.configurationsBaseData(r)
		data["Error"] = err.Error()
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}

	if err := h.applyRuntimeConfig(updated); err != nil {
		data := h.configurationsBaseData(r)
		data["Error"] = err.Error()
		h.renderTemplate(w, r, "configurations.html", data)
		return
	}

	msg := "Saved and applied configuration."
	if restartRequired {
		msg = "Saved and applied configuration (requires daemon restart)."
	}
	http.Redirect(w, r, "/configurations?msg="+url.QueryEscape(msg), http.StatusSeeOther)
}

// ConfigurationsArchiveProfiles shows the archive destination profiles management page.
func (h *Handler) ConfigurationsArchiveProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	profiles := h.archiveProfilesFromConfig(h.cfg)
	views := make([]map[string]any, 0, len(profiles))
	for i, p := range profiles {
		kind := "movie"
		if p.Kind == archive.KindSeries {
			kind = "series"
		}
		views = append(views, map[string]any{
			"Index":   i,
			"ID":      p.ID,
			"Name":    p.Name,
			"Kind":    kind,
			"BaseDir": p.BaseDir,
		})
	}
	h.renderTemplate(w, r, "archive_profiles.html", map[string]any{
		"Profiles":  views,
		"Derived":   len(h.cfg.Archive.Profiles) == 0,
		"NextIndex": len(views),
	})
}

// ConfigurationsArchiveProfilesSave persists archive destination profiles.
func (h *Handler) ConfigurationsArchiveProfilesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(h.configPath) == "" {
		http.Error(w, "No config path configured; cannot save.", http.StatusInternalServerError)
		return
	}

	updated := *h.cfg

	indices := r.PostForm["profile_indices"]
	// Keep indices stable and numeric where possible.
	maxIdx := 0
	cleanIdx := make([]string, 0, len(indices))
	for _, idx := range indices {
		idx = strings.TrimSpace(idx)
		if idx == "" {
			continue
		}
		cleanIdx = append(cleanIdx, idx)
		if n, err := strconv.Atoi(idx); err == nil {
			if n > maxIdx {
				maxIdx = n
			}
		}
	}
	indices = cleanIdx
	deleteIdx := make(map[string]bool)
	for _, idx := range indices {
		if r.PostFormValue("profile_delete_"+idx) == "on" {
			deleteIdx[idx] = true
		}
	}

	profiles := make([]config.ArchiveProfileConfig, 0, len(indices))
	for _, idx := range indices {
		if deleteIdx[idx] {
			continue
		}
		id := strings.TrimSpace(r.PostFormValue("profile_id_" + idx))
		name := strings.TrimSpace(r.PostFormValue("profile_name_" + idx))
		kind := strings.TrimSpace(r.PostFormValue("profile_kind_" + idx))
		baseDir := strings.TrimSpace(r.PostFormValue("profile_base_dir_" + idx))
		// Skip completely empty rows (e.g. user clicked add but didn't fill).
		if id == "" && name == "" && kind == "" && baseDir == "" {
			continue
		}
		profiles = append(profiles, config.ArchiveProfileConfig{ID: id, Name: name, Kind: kind, BaseDir: baseDir})
	}
	updated.Archive.Profiles = profiles

	if err := updated.Validate(); err != nil {
		// Re-render with the submitted values.
		views := make([]map[string]any, 0, len(indices))
		for _, idx := range indices {
			views = append(views, map[string]any{
				"Index":   idx,
				"ID":      r.PostFormValue("profile_id_" + idx),
				"Name":    r.PostFormValue("profile_name_" + idx),
				"Kind":    r.PostFormValue("profile_kind_" + idx),
				"BaseDir": r.PostFormValue("profile_base_dir_" + idx),
				"Delete":  deleteIdx[idx],
			})
		}
		h.renderTemplate(w, r, "archive_profiles.html", map[string]any{
			"Profiles":  views,
			"Derived":   false,
			"NextIndex": maxIdx + 1,
			"Error":     err.Error(),
		})
		return
	}

	if err := updated.Save(h.configPath); err != nil {
		h.renderTemplate(w, r, "archive_profiles.html", map[string]any{
			"Profiles":  nil,
			"Derived":   false,
			"NextIndex": 0,
			"Error":     err.Error(),
		})
		return
	}
	if err := h.applyRuntimeConfig(&updated); err != nil {
		h.renderTemplate(w, r, "archive_profiles.html", map[string]any{
			"Profiles":  nil,
			"Derived":   false,
			"NextIndex": 0,
			"Error":     err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/configurations?msg="+url.QueryEscape("Saved archive profiles."), http.StatusSeeOther)
}

func serverRestartRequired(oldCfg config.ServerConfig, newCfg config.ServerConfig) bool {
	if oldCfg.Host != newCfg.Host || oldCfg.Port != newCfg.Port {
		return true
	}
	if oldCfg.ReadTimeout != newCfg.ReadTimeout || oldCfg.WriteTimeout != newCfg.WriteTimeout {
		return true
	}
	if oldCfg.MaxHeaderBytes != newCfg.MaxHeaderBytes {
		return true
	}
	if oldCfg.TLS.Enabled != newCfg.TLS.Enabled {
		return true
	}
	if oldCfg.TLS.CertFile != newCfg.TLS.CertFile || oldCfg.TLS.KeyFile != newCfg.TLS.KeyFile {
		return true
	}
	return false
}

func (h *Handler) buildConfigFromForm(form url.Values) (*config.Config, error) {
	// Start from current config and apply supported fields.
	updated := *h.cfg

	// Server
	if v := strings.TrimSpace(form.Get("server_host")); v != "" {
		updated.Server.Host = v
	}
	if v := strings.TrimSpace(form.Get("server_port")); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid server port")
		}
		updated.Server.Port = p
	}
	if v := strings.TrimSpace(form.Get("server_read_timeout")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid server read timeout")
		}
		updated.Server.ReadTimeout = d
	}
	if v := strings.TrimSpace(form.Get("server_write_timeout")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid server write timeout")
		}
		updated.Server.WriteTimeout = d
	}
	if v := strings.TrimSpace(form.Get("server_max_header_bytes")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid server max header bytes")
		}
		updated.Server.MaxHeaderBytes = n
	}

	updated.Server.TLS.Enabled = form.Get("server_tls_enabled") == "on"
	if v := strings.TrimSpace(form.Get("server_tls_cert_file")); v != "" {
		updated.Server.TLS.CertFile = v
	}
	if v := strings.TrimSpace(form.Get("server_tls_key_file")); v != "" {
		updated.Server.TLS.KeyFile = v
	}

	// UI theme default
	if v := strings.TrimSpace(form.Get("ui_theme")); v != "" {
		updated.UI.Theme = v
	}
	if v := strings.TrimSpace(form.Get("ui_login_page")); v != "" {
		updated.UI.LoginPage = v
	}

	// VDR connection
	if v := strings.TrimSpace(form.Get("vdr_dvb_cards")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid vdr dvb cards")
		}
		updated.VDR.DVBCards = n
	}
	updated.VDR.WantedChannels = append([]string(nil), form["vdr_wanted_channels"]...)
	if v := strings.TrimSpace(form.Get("vdr_host")); v != "" {
		updated.VDR.Host = v
	}
	if v := strings.TrimSpace(form.Get("vdr_port")); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid vdr port")
		}
		updated.VDR.Port = p
	}
	if v := strings.TrimSpace(form.Get("vdr_timeout")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid vdr timeout")
		}
		updated.VDR.Timeout = d
	}
	if v := strings.TrimSpace(form.Get("vdr_video_dir")); v != "" {
		updated.VDR.VideoDir = v
	}
	if v := strings.TrimSpace(form.Get("vdr_config_dir")); v != "" {
		updated.VDR.ConfigDir = v
	}
	if v := strings.TrimSpace(form.Get("vdr_reconnect_delay")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid vdr reconnect delay")
		}
		updated.VDR.ReconnectDelay = d
	}
	// Stream URL template (optional, can be empty)
	updated.VDR.StreamURLTemplate = strings.TrimSpace(form.Get("vdr_stream_url_template"))
	// Streamdev backend URL for HLS proxy (optional, can be empty)
	updated.VDR.StreamdevBackendURL = strings.TrimSpace(form.Get("vdr_streamdev_backend_url"))

	// Archive
	updated.Archive.BaseDir = strings.TrimSpace(form.Get("archive_base_dir"))
	updated.Archive.FFMpegArgs = strings.TrimSpace(form.Get("archive_ffmpeg_args"))

	// Cache
	if v := strings.TrimSpace(form.Get("cache_epg_expiry")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid epg cache expiry")
		}
		updated.Cache.EPGExpiry = d
	}
	if v := strings.TrimSpace(form.Get("cache_recording_expiry")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid recording cache expiry")
		}
		updated.Cache.RecordingExpiry = d
	}

	// Auth
	updated.Auth.Enabled = form.Get("auth_enabled") == "on"
	if v := strings.TrimSpace(form.Get("auth_admin_user")); v != "" {
		updated.Auth.AdminUser = v
	}
	if v := strings.TrimSpace(form.Get("auth_admin_pass")); v != "" {
		updated.Auth.AdminPass = v
	}
	updated.Auth.GuestEnabled = form.Get("auth_guest_enabled") == "on"
	if v := strings.TrimSpace(form.Get("auth_guest_user")); v != "" {
		updated.Auth.GuestUser = v
	}
	if v := strings.TrimSpace(form.Get("auth_guest_pass")); v != "" {
		updated.Auth.GuestPass = v
	}
	updated.Auth.LocalNets = parseLines(form.Get("auth_local_nets"))

	// Timer defaults
	if v := strings.TrimSpace(form.Get("timer_default_priority")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid default priority")
		}
		updated.Timer.DefaultPriority = n
	}
	if v := strings.TrimSpace(form.Get("timer_default_lifetime")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid default lifetime")
		}
		updated.Timer.DefaultLifetime = n
	}
	if v := strings.TrimSpace(form.Get("timer_default_margin_start")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid default margin start")
		}
		updated.Timer.DefaultMarginStart = n
	}
	if v := strings.TrimSpace(form.Get("timer_default_margin_end")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid default margin end")
		}
		updated.Timer.DefaultMarginEnd = n
	}

	if err := updated.Validate(); err != nil {
		return nil, err
	}

	return &updated, nil
}

func parseLines(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (h *Handler) applyRuntimeConfig(updated *config.Config) error {
	// Update the shared config pointer in-place (middleware keeps working).
	*h.cfg = *updated

	// Apply pieces that can take effect immediately.
	h.SetUIThemeDefault(h.cfg.UI.Theme)
	if h.epgService != nil {
		h.epgService.SetCacheExpiry(h.cfg.Cache.EPGExpiry)
		h.epgService.SetWantedChannels(h.cfg.VDR.WantedChannels)
		h.epgService.InvalidateAllCaches()
	}
	if h.recordingService != nil {
		h.recordingService.SetCacheExpiry(h.cfg.Cache.RecordingExpiry)
	}

	// Update SVDRP connection settings (best-effort).
	if h.vdrClient != nil {
		if u, ok := h.vdrClient.(interface {
			UpdateConnection(host string, port int, timeout time.Duration)
		}); ok {
			u.UpdateConnection(h.cfg.VDR.Host, h.cfg.VDR.Port, h.cfg.VDR.Timeout)
		}
	}

	// Server host/port/timeouts can't be changed without restarting the process.
	return nil
}

// PlayingToday shows what each channel plays for a single day.
// It is a replacement for the old /epg list which could be too large.
func (h *Handler) PlayingToday(w http.ResponseWriter, r *http.Request) {
	loc := time.Now().Location()
	dayStart, err := parseDayParam(r, loc)
	if err != nil {
		h.logger.Error("invalid day parameter", slog.Any("error", err))
		dayStart = time.Now().In(loc)
		dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, loc)
	}
	dayEnd := dayStart.Add(24 * time.Hour)

	data := map[string]any{}
	data["Day"] = dayStart
	data["PrevDay"] = dayStart.Add(-24 * time.Hour).Format("2006-01-02")
	data["NextDay"] = dayStart.Add(24 * time.Hour).Format("2006-01-02")

	channels, err := h.epgService.GetChannels(r.Context())
	if err != nil {
		h.logger.Error("channels fetch error", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["ChannelGroups"] = []playingChannelGroup{}
		h.renderTemplate(w, r, "playing.html", data)
		return
	}

	// Pre-compute channel mappings needed for timer overlap lookups.
	numberByID := make(map[string]int, len(channels))
	idByNumber := make(map[int]string, len(channels))
	for _, ch := range channels {
		if ch.ID != "" {
			numberByID[ch.ID] = ch.Number
		}
		if ch.Number > 0 && ch.ID != "" {
			idByNumber[ch.Number] = ch.ID
		}
	}

	allEvents, err := h.epgService.GetEPG(r.Context(), "", dayStart)
	if err != nil {
		h.logger.Error("EPG fetch error on playing today", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["ChannelGroups"] = []playingChannelGroup{}
		h.renderTemplate(w, r, "playing.html", data)
		return
	}

	// Build an index of active timer occurrences for the selected day window.
	occByChannelNumber := map[int][]timerOccurrence{}
	occByChannelID := map[string][]timerOccurrence{}
	timersByID := map[int]domain.Timer{}
	if h.timerService != nil {
		timers, tErr := h.timerService.GetAllTimers(r.Context())
		if tErr != nil {
			h.logger.Warn("timers fetch error for playing today", slog.Any("error", tErr))
		} else {
			from := dayStart.Add(-24 * time.Hour)
			to := dayEnd.Add(24 * time.Hour)
			for _, t := range timers {
				// Disable "Record" if any timer exists for the event window, even if inactive.
				if strings.TrimSpace(t.ChannelID) == "" {
					continue
				}
				timersByID[t.ID] = t

				occs := timerOccurrences(t, from, to)
				if len(occs) == 0 {
					continue
				}
				occByChannelID[t.ChannelID] = append(occByChannelID[t.ChannelID], occs...)

				timerChNum := 0
				if n, err := strconv.Atoi(strings.TrimSpace(t.ChannelID)); err == nil {
					timerChNum = n
				} else if n := numberByID[t.ChannelID]; n > 0 {
					timerChNum = n
				}
				if timerChNum > 0 {
					occByChannelNumber[timerChNum] = append(occByChannelNumber[timerChNum], occs...)
				}
			}

			for ch := range occByChannelID {
				occs := occByChannelID[ch]
				sort.SliceStable(occs, func(i, j int) bool {
					if occs[i].Start.Equal(occs[j].Start) {
						return occs[i].TimerID < occs[j].TimerID
					}
					return occs[i].Start.Before(occs[j].Start)
				})
				occByChannelID[ch] = occs
			}
			for n := range occByChannelNumber {
				occs := occByChannelNumber[n]
				sort.SliceStable(occs, func(i, j int) bool {
					if occs[i].Start.Equal(occs[j].Start) {
						return occs[i].TimerID < occs[j].TimerID
					}
					return occs[i].Start.Before(occs[j].Start)
				})
				occByChannelNumber[n] = occs
			}
		}
	}

	eventsByChannel := make(map[string][]domain.EPGEvent)
	for i := range allEvents {
		// Keep events that overlap with the selected day.
		if allEvents[i].Start.Before(dayEnd) && allEvents[i].Stop.After(dayStart) {
			eventsByChannel[allEvents[i].ChannelID] = append(eventsByChannel[allEvents[i].ChannelID], allEvents[i])
		}
	}

	hasOverlappingActiveTimer := func(ev domain.EPGEvent) bool {
		_, ok := scheduledTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID)
		return ok
	}

	groups := make([]playingChannelGroup, 0, len(channels))
	jump := make([]playingChannelJumpOption, 0, len(channels))
	for _, ch := range channels {
		ev := eventsByChannel[ch.ID]
		if len(ev) == 0 {
			continue
		}

		anchor := fmt.Sprintf("ch-%d", ch.Number)
		if ch.Number <= 0 {
			anchor = fmt.Sprintf("ch-i-%d", len(groups)+1)
		}
		sort.SliceStable(ev, func(i, j int) bool {
			if !ev[i].Start.Equal(ev[j].Start) {
				return ev[i].Start.Before(ev[j].Start)
			}
			return ev[i].EventID < ev[j].EventID
		})

		views := make([]playingEventView, 0, len(ev))
		for _, e := range ev {
			// Some VDR/SVDRP outputs omit channel metadata on EPG items.
			// Normalize to improve timer overlap detection.
			if strings.TrimSpace(e.ChannelID) == "" {
				e.ChannelID = ch.ID
			}
			if e.ChannelNumber <= 0 {
				e.ChannelNumber = ch.Number
			}
			if strings.TrimSpace(e.ChannelName) == "" {
				e.ChannelName = ch.Name
			}
			views = append(views, playingEventView{EPGEvent: e, TimerActive: hasOverlappingActiveTimer(e)})
		}
		groups = append(groups, playingChannelGroup{Channel: ch, Anchor: anchor, Events: views})
		jump = append(jump, playingChannelJumpOption{Anchor: anchor, Label: ch.Name})
	}

	data["ChannelGroups"] = groups
	data["ChannelJump"] = jump
	h.renderTemplate(w, r, "playing.html", data)
}

// EPGList shows EPG listing
func (h *Handler) EPGList(w http.ResponseWriter, r *http.Request) {
	// Backward-compatible alias. The old /epg page could generate huge responses.
	day := r.URL.Query().Get("day")
	if day != "" {
		http.Redirect(w, r, "/playing?day="+day, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/playing", http.StatusFound)
}

// EventInfo renders a small popup page with detailed EPG information for a single event.
func (h *Handler) EventInfo(w http.ResponseWriter, r *http.Request) {
	eventID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if eventID <= 0 {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}

	channelID := r.URL.Query().Get("channel")

	// Event IDs are not guaranteed to be globally unique across channels.
	// Prefer a channel-scoped lookup when a channel id is provided.
	var events []domain.EPGEvent
	var err error
	if channelID != "" {
		events, err = h.epgService.GetEPG(r.Context(), channelID, time.Time{})
	} else {
		events, err = h.epgService.GetEPG(r.Context(), "", time.Time{})
	}
	if err != nil {
		h.logger.Error("EPG fetch error for event", slog.Any("error", err), slog.Int("event_id", eventID))
		handle := map[string]any{"HomeError": err.Error()}
		h.renderTemplate(w, r, "event.html", handle)
		return
	}

	var found *domain.EPGEvent
	for i := range events {
		if events[i].EventID != eventID {
			continue
		}
		if channelID != "" {
			if events[i].ChannelID == channelID {
				found = &events[i]
				break
			}
			continue
		}
		// Best-effort fallback when no channel id is provided.
		found = &events[i]
		break
	}
	if found == nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Event": found,
	}
	h.renderTemplate(w, r, "event.html", data)
}

// EPGSearch handles EPG search
func (h *Handler) EPGSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	isHTMX := r.Header.Get("HX-Request") == "true"
	if query == "" {
		if isHTMX {
			// HTMX updates the #search-results container; return results-only markup.
			h.renderTemplate(w, r, "search_results.html", nil)
			return
		}
		h.renderTemplate(w, r, "search.html", nil)
		return
	}

	events, err := h.epgService.SearchEPG(r.Context(), query)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	type searchEventView struct {
		domain.EPGEvent
		TimerActive bool
	}

	type searchDayGroup struct {
		DayLabel string
		Events   []searchEventView
	}

	views := make([]searchEventView, 0, len(events))
	if len(events) == 0 || h.timerService == nil {
		for _, ev := range events {
			views = append(views, searchEventView{EPGEvent: ev})
		}
	} else {
		// Build channel mappings needed for timer overlap lookups.
		channels, chErr := h.epgService.GetChannels(r.Context())
		numberByID := make(map[string]int, len(channels))
		idByNumber := make(map[int]string, len(channels))
		nameByID := make(map[string]string, len(channels))
		if chErr == nil {
			for _, ch := range channels {
				if ch.ID != "" {
					numberByID[ch.ID] = ch.Number
					nameByID[ch.ID] = ch.Name
				}
				if ch.Number > 0 && ch.ID != "" {
					idByNumber[ch.Number] = ch.ID
				}
			}
		}

		// Determine a time window that covers the search results (+ margin for recurring timers).
		minStart := events[0].Start
		maxStop := events[0].Stop
		for i := 1; i < len(events); i++ {
			if events[i].Start.Before(minStart) {
				minStart = events[i].Start
			}
			if maxStop.Before(events[i].Stop) {
				maxStop = events[i].Stop
			}
		}
		from := minStart.Add(-24 * time.Hour)
		to := maxStop.Add(24 * time.Hour)
		if from.After(to) {
			from, to = to, from
		}

		occByChannelNumber := map[int][]timerOccurrence{}
		occByChannelID := map[string][]timerOccurrence{}
		timersByID := map[int]domain.Timer{}
		if timers, tErr := h.timerService.GetAllTimers(r.Context()); tErr == nil {
			for _, t := range timers {
				// Disable "Record" if any timer exists for the event window, even if inactive.
				if strings.TrimSpace(t.ChannelID) == "" {
					continue
				}
				timersByID[t.ID] = t

				occs := timerOccurrences(t, from, to)
				if len(occs) == 0 {
					continue
				}
				occByChannelID[t.ChannelID] = append(occByChannelID[t.ChannelID], occs...)

				timerChNum := 0
				if n, err := strconv.Atoi(strings.TrimSpace(t.ChannelID)); err == nil {
					timerChNum = n
				} else if n := numberByID[t.ChannelID]; n > 0 {
					timerChNum = n
				}
				if timerChNum > 0 {
					occByChannelNumber[timerChNum] = append(occByChannelNumber[timerChNum], occs...)
				}
			}

			for ch := range occByChannelID {
				occs := occByChannelID[ch]
				sort.SliceStable(occs, func(i, j int) bool {
					if occs[i].Start.Equal(occs[j].Start) {
						return occs[i].TimerID < occs[j].TimerID
					}
					return occs[i].Start.Before(occs[j].Start)
				})
				occByChannelID[ch] = occs
			}
			for n := range occByChannelNumber {
				occs := occByChannelNumber[n]
				sort.SliceStable(occs, func(i, j int) bool {
					if occs[i].Start.Equal(occs[j].Start) {
						return occs[i].TimerID < occs[j].TimerID
					}
					return occs[i].Start.Before(occs[j].Start)
				})
				occByChannelNumber[n] = occs
			}
		} else {
			h.logger.Warn("timers fetch error for search", slog.Any("error", tErr))
		}

		loc := time.Now().Location()
		hasOverlappingTimer := func(ev domain.EPGEvent) bool {
			_, ok := scheduledTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID)
			return ok
		}

		for _, ev := range events {
			// Normalize missing channel metadata for better overlap detection.
			if strings.TrimSpace(ev.ChannelID) == "" && ev.ChannelNumber > 0 {
				if id := idByNumber[ev.ChannelNumber]; id != "" {
					ev.ChannelID = id
				}
			}
			if ev.ChannelNumber <= 0 && ev.ChannelID != "" {
				ev.ChannelNumber = numberByID[ev.ChannelID]
			}
			if strings.TrimSpace(ev.ChannelName) == "" && ev.ChannelID != "" {
				ev.ChannelName = nameByID[ev.ChannelID]
			}
			views = append(views, searchEventView{EPGEvent: ev, TimerActive: hasOverlappingTimer(ev)})
		}
	}

	// Group results by local day, like the /epgsearch results view.
	loc := time.Local
	sort.SliceStable(views, func(i, j int) bool {
		ai := views[i]
		aj := views[j]
		if !ai.Start.Equal(aj.Start) {
			return ai.Start.Before(aj.Start)
		}
		// Tie-breakers for stable rendering.
		if ai.ChannelName != aj.ChannelName {
			return ai.ChannelName < aj.ChannelName
		}
		if ai.Title != aj.Title {
			return ai.Title < aj.Title
		}
		return ai.EventID < aj.EventID
	})
	dayGroups := make([]searchDayGroup, 0)
	var currentDay time.Time
	currentIdx := -1
	for _, v := range views {
		s := v.Start.In(loc)
		day := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
		if currentIdx == -1 || !day.Equal(currentDay) {
			dayGroups = append(dayGroups, searchDayGroup{DayLabel: day.Format("Mon 2006-01-02")})
			currentDay = day
			currentIdx = len(dayGroups) - 1
		}
		dayGroups[currentIdx].Events = append(dayGroups[currentIdx].Events, v)
	}

	data := map[string]any{
		"Query":     query,
		"DayGroups": dayGroups,
	}

	if isHTMX {
		h.renderTemplate(w, r, "search_results.html", data)
		return
	}

	h.renderTemplate(w, r, "search.html", data)
}

type epgSearchView struct {
	config.EPGSearch
	ChannelLabel string
	FromLabel    string
	ToLabel      string
}

// EPGSearchList shows saved EPG searches and allows executing selected searches.
func (h *Handler) EPGSearchList(w http.ResponseWriter, r *http.Request) {
	searches := []config.EPGSearch{}
	if h.cfg != nil {
		searches = append([]config.EPGSearch(nil), h.cfg.EPG.Searches...)
	}

	channels, chErr := h.epgService.GetChannels(r.Context())
	nameByID := map[string]string{}
	for _, ch := range channels {
		if ch.ID != "" {
			nameByID[ch.ID] = ch.Name
		}
	}

	views := make([]epgSearchView, 0, len(searches))
	for _, s := range searches {
		label := "-"
		switch s.UseChannel {
		case "single":
			if n := nameByID[s.ChannelID]; n != "" {
				label = n
			} else if s.ChannelID != "" {
				label = s.ChannelID
			}
		case "range":
			from := nameByID[s.ChannelFrom]
			to := nameByID[s.ChannelTo]
			if from != "" && to != "" {
				label = from + " - " + to
			} else if s.ChannelFrom != "" || s.ChannelTo != "" {
				label = strings.TrimSpace(s.ChannelFrom + " - " + s.ChannelTo)
			}
		}

		views = append(views, epgSearchView{
			EPGSearch:    s,
			ChannelLabel: label,
			FromLabel:    "--:--",
			ToLabel:      "--:--",
		})
	}

	data := map[string]any{
		"Searches": views,
	}
	if chErr != nil {
		h.logger.Warn("channels fetch error for epgsearch", slog.Any("error", chErr))
		data["HomeError"] = chErr.Error()
	}

	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		data["Message"] = msg
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("err")); errMsg != "" {
		data["Error"] = errMsg
	}

	h.renderTemplate(w, r, "epgsearch.html", data)
}

type epgSearchResultEventView struct {
	domain.EPGEvent
	TimerActive bool
	TimerLabel  string
}

type epgSearchResultDayGroup struct {
	DayLabel string
	Events   []epgSearchResultEventView
}

func overlappingTimerForEvent(
	ev domain.EPGEvent,
	loc *time.Location,
	occByChannelNumber map[int][]timerOccurrence,
	occByChannelID map[string][]timerOccurrence,
	numberByID map[string]int,
	idByNumber map[int]string,
	timersByID map[int]domain.Timer,
) (domain.Timer, bool) {
	if loc == nil {
		loc = time.Local
	}

	lookupOccs := func() []timerOccurrence {
		if ev.ChannelNumber > 0 {
			if occs := occByChannelNumber[ev.ChannelNumber]; len(occs) > 0 {
				return occs
			}
		}
		if ev.ChannelID != "" {
			if ev.ChannelNumber <= 0 {
				if n := numberByID[ev.ChannelID]; n > 0 {
					if occs := occByChannelNumber[n]; len(occs) > 0 {
						return occs
					}
				}
			}
			if occs := occByChannelID[ev.ChannelID]; len(occs) > 0 {
				return occs
			}
			if ev.ChannelNumber > 0 {
				if occs := occByChannelID[strconv.Itoa(ev.ChannelNumber)]; len(occs) > 0 {
					return occs
				}
			}
		}
		if ev.ChannelNumber > 0 {
			if id := idByNumber[ev.ChannelNumber]; id != "" {
				if occs := occByChannelID[id]; len(occs) > 0 {
					return occs
				}
			}
		}
		return nil
	}

	occs := lookupOccs()
	if len(occs) == 0 {
		return domain.Timer{}, false
	}

	evStart := ev.Start.In(loc)
	evStop := ev.Stop.In(loc)
	if evStop.Before(evStart) {
		evStop = evStart
	}

	// Guard against falsely marking adjacent programs as scheduled due to small
	// recording margins. Require that the overlap covers most of the event.
	evDur := evStop.Sub(evStart)
	if evDur <= 0 {
		return domain.Timer{}, false
	}

	overlapEnough := func(aStart, aStop, bStart, bStop time.Time) bool {
		start := aStart
		if bStart.After(start) {
			start = bStart
		}
		stop := aStop
		if bStop.Before(stop) {
			stop = bStop
		}
		overlap := stop.Sub(start)
		if overlap <= 0 {
			return false
		}
		// overlap >= 60% of event duration
		return overlap*5 >= evDur*3
	}

	for _, occ := range occs {
		if !occ.Start.Before(evStop) || !evStart.Before(occ.Stop) {
			continue
		}
		if !overlapEnough(evStart, evStop, occ.Start.In(loc), occ.Stop.In(loc)) {
			continue
		}
		t, ok := timersByID[occ.TimerID]
		if !ok {
			continue
		}
		return t, true
	}

	return domain.Timer{}, false
}

// EPGSearchExecute runs selected searches and renders matching events.
func (h *Handler) EPGSearchExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	idsRaw := r.PostForm["ids"]

	searchByID := map[int]config.EPGSearch{}
	activeSearches := make([]config.EPGSearch, 0, 16)
	if h.cfg != nil {
		activeSearches = make([]config.EPGSearch, 0, len(h.cfg.EPG.Searches))
		for _, s := range h.cfg.EPG.Searches {
			searchByID[s.ID] = s
			if s.Active {
				activeSearches = append(activeSearches, s)
			}
		}
	}

	selected := make([]config.EPGSearch, 0, len(idsRaw))
	if len(idsRaw) == 0 {
		// No explicit selection means: execute all *active* searches.
		selected = append(selected, activeSearches...)
	}
	for _, raw := range idsRaw {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || id <= 0 {
			continue
		}
		s, ok := searchByID[id]
		if !ok {
			continue
		}
		if !s.Active {
			continue
		}
		selected = append(selected, s)
	}
	if len(selected) == 0 {
		http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape("No active searches configured."), http.StatusSeeOther)
		return
	}

	channels, _ := h.epgService.GetChannels(r.Context())
	order := make(map[string]int, len(channels))
	nameByID := make(map[string]string, len(channels))
	numberByID := make(map[string]int, len(channels))
	idByNumber := make(map[int]string, len(channels))
	for i, ch := range channels {
		if ch.ID != "" {
			order[ch.ID] = i + 1
			nameByID[ch.ID] = ch.Name
			numberByID[ch.ID] = ch.Number
		}
		if ch.Number > 0 && ch.ID != "" {
			idByNumber[ch.Number] = ch.ID
		}
	}

	allEvents, err := h.epgService.GetEPG(r.Context(), "", time.Time{})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	// Lookup existing active timers by channel + time overlap.
	// NOTE: SVDRP LSTT output doesn't include an EPG EventID, so matching by (ChannelID, EventID)
	// doesn't work reliably. Instead, treat an event as "already scheduled" when any active timer
	// on the same channel overlaps the event time window (timers usually include margins).
	timersByID := map[int]domain.Timer{}
	occByChannelNumber := map[int][]timerOccurrence{}
	occByChannelID := map[string][]timerOccurrence{}
	if h.timerService != nil {
		timers, tErr := h.timerService.GetAllTimers(r.Context())
		if tErr != nil {
			h.logger.Warn("timers fetch error for epgsearch results", slog.Any("error", tErr))
		} else {
			for _, t := range timers {
				if strings.TrimSpace(t.ChannelID) == "" {
					continue
				}
				timersByID[t.ID] = t
			}
		}
	}

	// Merge matches from all selected searches; de-duplicate by ChannelID+EventID.
	seen := map[string]bool{}
	combined := make([]domain.EPGEvent, 0, 128)
	for _, s := range selected {
		matches, err := services.ExecuteSavedEPGSearch(allEvents, s, order)
		if err != nil {
			http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape("Invalid search: "+err.Error()), http.StatusSeeOther)
			return
		}
		for _, ev := range matches {
			key := fmt.Sprintf("%s:%d", ev.ChannelID, ev.EventID)
			if ev.EventID <= 0 {
				key = fmt.Sprintf("%s:%s", ev.ChannelID, ev.Start.UTC().Format(time.RFC3339Nano))
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			combined = append(combined, ev)
		}
	}

	loc := time.Local
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Build timer occurrences for the relevant event window.
	if len(timersByID) > 0 && len(combined) > 0 {
		minStart := combined[0].Start.In(loc)
		maxStop := combined[0].Stop.In(loc)
		for i := 1; i < len(combined); i++ {
			s := combined[i].Start.In(loc)
			e := combined[i].Stop.In(loc)
			if s.Before(minStart) {
				minStart = s
			}
			if e.After(maxStop) {
				maxStop = e
			}
		}
		from := minStart.Add(-24 * time.Hour)
		to := maxStop.Add(24 * time.Hour)
		for _, t := range timersByID {
			occs := timerOccurrences(t, from, to)
			// Always index by the raw timer channel field.
			if t.ChannelID != "" {
				occByChannelID[t.ChannelID] = append(occByChannelID[t.ChannelID], occs...)
			}

			// Additionally index by channel number when possible, because timers often
			// reference channels by number (e.g. "2159") while EPG events carry the
			// derived VDR channel id.
			timerChNum := 0
			if n, err := strconv.Atoi(strings.TrimSpace(t.ChannelID)); err == nil {
				timerChNum = n
			} else if n := numberByID[t.ChannelID]; n > 0 {
				timerChNum = n
			}
			if timerChNum > 0 {
				occByChannelNumber[timerChNum] = append(occByChannelNumber[timerChNum], occs...)
			}
		}
		for ch := range occByChannelID {
			occs := occByChannelID[ch]
			sort.SliceStable(occs, func(i, j int) bool {
				if occs[i].Start.Equal(occs[j].Start) {
					return occs[i].TimerID < occs[j].TimerID
				}
				return occs[i].Start.Before(occs[j].Start)
			})
			occByChannelID[ch] = occs
		}
		for n := range occByChannelNumber {
			occs := occByChannelNumber[n]
			sort.SliceStable(occs, func(i, j int) bool {
				if occs[i].Start.Equal(occs[j].Start) {
					return occs[i].TimerID < occs[j].TimerID
				}
				return occs[i].Start.Before(occs[j].Start)
			})
			occByChannelNumber[n] = occs
		}
	}

	sort.SliceStable(combined, func(i, j int) bool {
		a := combined[i].Start.In(loc)
		b := combined[j].Start.In(loc)

		aDay := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, loc)
		bDay := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, loc)

		aIsToday := aDay.Equal(today)
		bIsToday := bDay.Equal(today)
		if aIsToday != bIsToday {
			return aIsToday
		}

		aAfter := aDay.After(today)
		bAfter := bDay.After(today)
		if aAfter != bAfter {
			// Future results before past results.
			return aAfter
		}

		if !aDay.Equal(bDay) {
			if aAfter && bAfter {
				return aDay.Before(bDay)
			}
			// Past days: most recent first.
			return aDay.After(bDay)
		}

		if !a.Equal(b) {
			return a.Before(b)
		}
		return combined[i].ChannelNumber < combined[j].ChannelNumber
	})

	dayGroups := []epgSearchResultDayGroup{}
	var currentDay time.Time
	var currentIdx int
	for _, ev := range combined {
		start := ev.Start.In(loc)
		evDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
		if len(dayGroups) == 0 || !evDay.Equal(currentDay) {
			dayGroups = append(dayGroups, epgSearchResultDayGroup{
				DayLabel: start.Format("Monday, 01/02/2006"),
				Events:   []epgSearchResultEventView{},
			})
			currentDay = evDay
			currentIdx = len(dayGroups) - 1
		}

		label := "---"
		active := false
		if t, ok := overlappingTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID); ok {
			active = true
			if strings.TrimSpace(t.Title) != "" {
				label = t.Title
			} else {
				label = "active"
			}
		}

		dayGroups[currentIdx].Events = append(dayGroups[currentIdx].Events, epgSearchResultEventView{
			EPGEvent:    ev,
			TimerActive: active,
			TimerLabel:  label,
		})
	}

	data := map[string]any{
		"DayGroups": dayGroups,
	}
	h.renderTemplate(w, r, "epgsearch_results.html", data)
}

func (h *Handler) epgSearchFormData(r *http.Request, search config.EPGSearch) map[string]any {
	channels, err := h.epgService.GetChannels(r.Context())
	if err != nil {
		channels = []domain.Channel{}
	}
	data := map[string]any{
		"Search":   search,
		"Channels": channels,
	}
	return data
}

func (h *Handler) EPGSearchNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	search := config.EPGSearch{
		Active:     true,
		Mode:       "phrase",
		InTitle:    true,
		InSubtitle: true,
		InDesc:     true,
		UseChannel: "no",
	}
	data := h.epgSearchFormData(r, search)
	data["PageTitle"] = "Add New Search - VDRAdmin-go"
	data["Heading"] = "Add New Search"
	data["FormAction"] = "/epgsearch/new"
	h.renderTemplate(w, r, "epgsearch_edit.html", data)
}

func (h *Handler) renderEPGSearchRun(w http.ResponseWriter, r *http.Request, search config.EPGSearch, pageTitle, heading, formAction string) {
	config.NormalizeEPGSearch(&search)
	if err := config.ValidateEPGSearch(search); err != nil {
		data := h.epgSearchFormData(r, search)
		data["Error"] = "Invalid search: " + err.Error()
		data["PageTitle"] = pageTitle
		data["Heading"] = heading
		data["FormAction"] = formAction
		h.renderTemplate(w, r, "epgsearch_edit.html", data)
		return
	}

	channels, chErr := h.epgService.GetChannels(r.Context())
	if chErr != nil {
		channels = []domain.Channel{}
	}
	order := make(map[string]int, len(channels))
	nameByID := make(map[string]string, len(channels))
	numberByID := make(map[string]int, len(channels))
	idByNumber := make(map[int]string, len(channels))
	for i, ch := range channels {
		if ch.ID != "" {
			order[ch.ID] = i + 1
			nameByID[ch.ID] = ch.Name
			numberByID[ch.ID] = ch.Number
		}
		if ch.Number > 0 && ch.ID != "" {
			idByNumber[ch.Number] = ch.ID
		}
	}

	allEvents, err := h.epgService.GetEPG(r.Context(), "", time.Time{})
	if err != nil {
		data := h.epgSearchFormData(r, search)
		data["Error"] = err.Error()
		data["PageTitle"] = pageTitle
		data["Heading"] = heading
		data["FormAction"] = formAction
		h.renderTemplate(w, r, "epgsearch_edit.html", data)
		return
	}

	matches, err := services.ExecuteSavedEPGSearch(allEvents, search, order)
	if err != nil {
		data := h.epgSearchFormData(r, search)
		data["Error"] = "Invalid search: " + err.Error()
		data["PageTitle"] = pageTitle
		data["Heading"] = heading
		data["FormAction"] = formAction
		h.renderTemplate(w, r, "epgsearch_edit.html", data)
		return
	}

	// Build timer occurrences for the relevant event window.
	timersByID := map[int]domain.Timer{}
	occByChannelNumber := map[int][]timerOccurrence{}
	occByChannelID := map[string][]timerOccurrence{}
	if h.timerService != nil {
		timers, tErr := h.timerService.GetAllTimers(r.Context())
		if tErr != nil {
			h.logger.Warn("timers fetch error for epgsearch run", slog.Any("error", tErr))
		} else {
			for _, t := range timers {
				if strings.TrimSpace(t.ChannelID) == "" {
					continue
				}
				timersByID[t.ID] = t
			}
		}
	}

	loc := time.Local
	if len(timersByID) > 0 && len(matches) > 0 {
		minStart := matches[0].Start.In(loc)
		maxStop := matches[0].Stop.In(loc)
		for i := 1; i < len(matches); i++ {
			s := matches[i].Start.In(loc)
			e := matches[i].Stop.In(loc)
			if s.Before(minStart) {
				minStart = s
			}
			if e.After(maxStop) {
				maxStop = e
			}
		}
		from := minStart.Add(-24 * time.Hour)
		to := maxStop.Add(24 * time.Hour)
		for _, t := range timersByID {
			occs := timerOccurrences(t, from, to)
			if t.ChannelID != "" {
				occByChannelID[t.ChannelID] = append(occByChannelID[t.ChannelID], occs...)
			}
			timerChNum := 0
			if n, err := strconv.Atoi(strings.TrimSpace(t.ChannelID)); err == nil {
				timerChNum = n
			} else if n := numberByID[t.ChannelID]; n > 0 {
				timerChNum = n
			}
			if timerChNum > 0 {
				occByChannelNumber[timerChNum] = append(occByChannelNumber[timerChNum], occs...)
			}
		}
		for ch := range occByChannelID {
			occs := occByChannelID[ch]
			sort.SliceStable(occs, func(i, j int) bool {
				if occs[i].Start.Equal(occs[j].Start) {
					return occs[i].TimerID < occs[j].TimerID
				}
				return occs[i].Start.Before(occs[j].Start)
			})
			occByChannelID[ch] = occs
		}
		for n := range occByChannelNumber {
			occs := occByChannelNumber[n]
			sort.SliceStable(occs, func(i, j int) bool {
				if occs[i].Start.Equal(occs[j].Start) {
					return occs[i].TimerID < occs[j].TimerID
				}
				return occs[i].Start.Before(occs[j].Start)
			})
			occByChannelNumber[n] = occs
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		a := matches[i].Start.In(loc)
		b := matches[j].Start.In(loc)
		if !a.Equal(b) {
			return a.Before(b)
		}
		return matches[i].ChannelNumber < matches[j].ChannelNumber
	})

	dayGroups := []epgSearchResultDayGroup{}
	var currentDay time.Time
	var currentIdx int
	for _, ev := range matches {
		start := ev.Start.In(loc)
		evDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
		if len(dayGroups) == 0 || !evDay.Equal(currentDay) {
			dayGroups = append(dayGroups, epgSearchResultDayGroup{
				DayLabel: start.Format("Monday, 01/02/2006"),
				Events:   []epgSearchResultEventView{},
			})
			currentDay = evDay
			currentIdx = len(dayGroups) - 1
		}

		if ev.ChannelNumber <= 0 && strings.TrimSpace(ev.ChannelID) != "" {
			ev.ChannelNumber = numberByID[ev.ChannelID]
		}
		if strings.TrimSpace(ev.ChannelName) == "" && strings.TrimSpace(ev.ChannelID) != "" {
			ev.ChannelName = nameByID[ev.ChannelID]
		}
		_, active := overlappingTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID)

		dayGroups[currentIdx].Events = append(dayGroups[currentIdx].Events, epgSearchResultEventView{
			EPGEvent:    ev,
			TimerActive: active,
			TimerLabel:  "",
		})
	}

	data := h.epgSearchFormData(r, search)
	data["PageTitle"] = pageTitle
	data["Heading"] = heading
	data["FormAction"] = formAction
	data["RunDayGroups"] = dayGroups
	h.renderTemplate(w, r, "epgsearch_edit.html", data)
}

func (h *Handler) EPGSearchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.PostForm.Get("action"))

	search := parseEPGSearchFromForm(r.PostForm)

	// "Run" executes the search without saving and re-renders the form with matches.
	if action == "run" {
		h.renderEPGSearchRun(w, r, search, "Add New Search - VDRAdmin-go", "Add New Search", "/epgsearch/new")
		return
	}

	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}

	search.ID = nextEPGSearchID(h.cfg.EPG.Searches)

	updated := *h.cfg
	updated.EPG.Searches = append(append([]config.EPGSearch(nil), h.cfg.EPG.Searches...), search)
	if err := updated.Validate(); err != nil {
		data := h.epgSearchFormData(r, search)
		data["Error"] = err.Error()
		data["PageTitle"] = "Add New Search - VDRAdmin-go"
		data["Heading"] = "Add New Search"
		data["FormAction"] = "/epgsearch/new"
		h.renderTemplate(w, r, "epgsearch_edit.html", data)
		return
	}
	*h.cfg = updated
	if strings.TrimSpace(h.configPath) != "" {
		if err := h.cfg.Save(h.configPath); err != nil {
			http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/epgsearch?msg="+url.QueryEscape("Saved search."), http.StatusSeeOther)
}

func (h *Handler) EPGSearchEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		http.Error(w, "Invalid search id", http.StatusBadRequest)
		return
	}
	var found *config.EPGSearch
	for i := range h.cfg.EPG.Searches {
		if h.cfg.EPG.Searches[i].ID == id {
			found = &h.cfg.EPG.Searches[i]
			break
		}
	}
	if found == nil {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	data := h.epgSearchFormData(r, *found)
	data["PageTitle"] = "Edit Search - VDRAdmin-go"
	data["Heading"] = "Edit Search"
	data["FormAction"] = "/epgsearch/edit"
	h.renderTemplate(w, r, "epgsearch_edit.html", data)
}

func (h *Handler) EPGSearchUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.PostForm.Get("action"))

	search := parseEPGSearchFromForm(r.PostForm)
	if v := strings.TrimSpace(r.PostForm.Get("id")); v != "" {
		id, _ := strconv.Atoi(v)
		search.ID = id
	}
	if search.ID <= 0 {
		http.Error(w, "Invalid search id", http.StatusBadRequest)
		return
	}

	// "Run" executes the (edited) search without saving and re-renders the form with matches.
	if action == "run" {
		h.renderEPGSearchRun(w, r, search, "Edit Search - VDRAdmin-go", "Edit Search", "/epgsearch/edit")
		return
	}

	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}

	updated := *h.cfg
	updated.EPG.Searches = append([]config.EPGSearch(nil), h.cfg.EPG.Searches...)
	replaced := false
	for i := range updated.EPG.Searches {
		if updated.EPG.Searches[i].ID == search.ID {
			updated.EPG.Searches[i] = search
			replaced = true
			break
		}
	}
	if !replaced {
		http.Error(w, "Search not found", http.StatusNotFound)
		return
	}
	if err := updated.Validate(); err != nil {
		data := h.epgSearchFormData(r, search)
		data["Error"] = err.Error()
		data["PageTitle"] = "Edit Search - VDRAdmin-go"
		data["Heading"] = "Edit Search"
		data["FormAction"] = "/epgsearch/edit"
		h.renderTemplate(w, r, "epgsearch_edit.html", data)
		return
	}
	*h.cfg = updated
	if strings.TrimSpace(h.configPath) != "" {
		if err := h.cfg.Save(h.configPath); err != nil {
			http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/epgsearch?msg="+url.QueryEscape("Saved search."), http.StatusSeeOther)
}

func (h *Handler) EPGSearchDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	id, _ := strconv.Atoi(r.PostForm.Get("id"))
	if id <= 0 {
		http.Error(w, "Invalid search id", http.StatusBadRequest)
		return
	}

	updated := *h.cfg
	updated.EPG.Searches = make([]config.EPGSearch, 0, len(h.cfg.EPG.Searches))
	for _, s := range h.cfg.EPG.Searches {
		if s.ID == id {
			continue
		}
		updated.EPG.Searches = append(updated.EPG.Searches, s)
	}
	if err := updated.Validate(); err != nil {
		http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	*h.cfg = updated
	if strings.TrimSpace(h.configPath) != "" {
		if err := h.cfg.Save(h.configPath); err != nil {
			http.Redirect(w, r, "/epgsearch?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/epgsearch?msg="+url.QueryEscape("Deleted search."), http.StatusSeeOther)
}

func nextEPGSearchID(existing []config.EPGSearch) int {
	max := 0
	for _, s := range existing {
		if s.ID > max {
			max = s.ID
		}
	}
	return max + 1
}

func parseEPGSearchFromForm(form url.Values) config.EPGSearch {
	s := config.EPGSearch{}
	s.Active = form.Get("active") == "on"
	s.Pattern = strings.TrimSpace(form.Get("pattern"))
	s.Mode = strings.TrimSpace(form.Get("mode"))
	s.MatchCase = form.Get("match_case") == "on"
	s.InTitle = form.Get("in_title") == "on"
	s.InSubtitle = form.Get("in_subtitle") == "on"
	s.InDesc = form.Get("in_description") == "on"
	s.UseChannel = strings.TrimSpace(form.Get("use_channel"))
	s.ChannelID = strings.TrimSpace(form.Get("channel_id"))
	s.ChannelFrom = strings.TrimSpace(form.Get("channel_from"))
	s.ChannelTo = strings.TrimSpace(form.Get("channel_to"))
	return s
}

type timerTimelineDayOption struct {
	Value string
	Label string
}

type timerTimelineBlock struct {
	Title      string
	StartLabel string
	StopLabel  string
	LeftPct    float64
	WidthPct   float64
	Class      string
}

type timerTimelineRow struct {
	ChannelName string
	Blocks      []timerTimelineBlock
}

// TimerList shows all timers
func (h *Handler) TimerList(w http.ResponseWriter, r *http.Request) {
	loc := time.Local

	localNow := h.now().In(loc)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)

	// Keep collision/critical computation stable (do not shift based on UI day selection).
	collisionWindowFrom := todayStart
	collisionWindowTo := collisionWindowFrom.Add(8 * 24 * time.Hour) // cover at least one full week for recurring timers

	// Parse selected day (may be empty/invalid or outside the available days).
	selectedDay := strings.TrimSpace(r.URL.Query().Get("day"))
	selectedDayStart := todayStart
	if selectedDay != "" {
		if t, err := time.ParseInLocation("2006-01-02", selectedDay, loc); err == nil {
			selectedDayStart = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
	}
	selectedDayEnd := selectedDayStart.Add(24 * time.Hour)

	// Build day dropdown options from active timer occurrences, but never include days
	// in the past ("already gone"). Keep the horizon stable so it doesn't jump.
	optionsFrom := todayStart
	optionsTo := todayStart.Add(180 * 24 * time.Hour)
	if selectedDayStart.After(optionsTo) {
		optionsTo = selectedDayStart.Add(30 * 24 * time.Hour)
	}

	timers, err := h.timerService.GetAllTimers(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	channels, chErr := h.epgService.GetChannels(r.Context())
	nameByID := map[string]string{}
	orderByID := map[string]int{}
	channelByNumber := map[int]domain.Channel{}
	if chErr == nil {
		for i, ch := range channels {
			if ch.ID != "" {
				nameByID[ch.ID] = ch.Name
				orderByID[ch.ID] = i + 1
			}
			if ch.Number != 0 {
				nameByID[strconv.Itoa(ch.Number)] = ch.Name
				channelByNumber[ch.Number] = ch
			}
		}
	}

	type timerView struct {
		domain.Timer
		ChannelName     string
		IsRecording     bool
		IsCritical      bool
		IsCollision     bool
		NextOccurrences []string
	}

	views := make([]timerView, 0, len(timers))
	for _, t := range timers {
		name := nameByID[t.ChannelID]
		if name == "" {
			name = t.ChannelID
		}
		isRec := false
		if !t.Start.IsZero() && !t.Stop.IsZero() {
			isRec = (t.Start.Before(localNow) || t.Start.Equal(localNow)) && t.Stop.After(localNow)
		}

		// For recurring timers, show all upcoming occurrences within the next-week horizon.
		// This matches user expectations for weekday masks (e.g. Thu+Fri at midnight).
		var nextOcc []string
		if t.Active && isWeekdayMaskHTTP(t.DaySpec) {
			occFrom := todayStart
			occTo := todayStart.Add(8 * 24 * time.Hour)
			for _, occ := range timerOccurrences(t, occFrom, occTo) {
				// Keep occurrences that haven't fully ended yet.
				if !occ.Stop.After(localNow) {
					continue
				}
				s := occ.Start.In(loc).Format("2006-01-02 15:04") + " - " + occ.Stop.In(loc).Format("15:04")
				nextOcc = append(nextOcc, s)
			}
		}

		views = append(views, timerView{Timer: t, ChannelName: name, IsRecording: isRec, NextOccurrences: nextOcc})
	}

	// Mark overlapping timers (yellow) and critical timers (red) based on configured DVB cards.
	dvbCards := 1
	if h.cfg != nil && h.cfg.VDR.DVBCards > 0 {
		dvbCards = h.cfg.VDR.DVBCards
	}

	collisionIDs, criticalIDs := timerOverlapStates(timers, dvbCards, collisionWindowFrom, collisionWindowTo, func(t domain.Timer) string {
		return transponderKeyForTimer(t, channels)
	})
	for i := range views {
		if collisionIDs[views[i].ID] {
			views[i].IsCollision = true
		}
		if criticalIDs[views[i].ID] {
			views[i].IsCritical = true
		}
	}

	now := h.now()
	sort.SliceStable(views, func(i, j int) bool {
		si := views[i].Start
		sj := views[j].Start
		if si.IsZero() != sj.IsZero() {
			return !si.IsZero()
		}
		if si.IsZero() && sj.IsZero() {
			return views[i].ID < views[j].ID
		}

		ranki := 2
		rankj := 2
		if views[i].IsRecording {
			ranki = 0
		} else if si.After(now) {
			ranki = 1
		}
		if views[j].IsRecording {
			rankj = 0
		} else if sj.After(now) {
			rankj = 1
		}
		if ranki != rankj {
			return ranki < rankj
		}

		// Within each category, order by start time.
		if ranki == 2 {
			// Past: most recent first.
			if !si.Equal(sj) {
				return si.After(sj)
			}
		} else {
			// Recording/upcoming: earliest first.
			if !si.Equal(sj) {
				return si.Before(sj)
			}
		}
		return views[i].ID < views[j].ID
	})

	// Build timeline day options from active timer occurrences in the stable horizon.
	daysByValue := map[string]time.Time{}
	recurringHorizonTo := todayStart.Add(8 * 24 * time.Hour)
	for _, t := range timers {
		if !t.Active {
			continue
		}
		occFrom := optionsFrom
		occTo := optionsTo
		if isWeekdayMaskHTTP(t.DaySpec) {
			occFrom = todayStart
			occTo = recurringHorizonTo
		}
		for _, occ := range timerOccurrences(t, occFrom, occTo) {
			start := occ.Start.In(loc)
			stop := occ.Stop.In(loc)
			if stop.Before(start) {
				stop = start
			}
			// Only include days with actual timer time. In particular, a timer that ends
			// exactly at 00:00 should not make the next day selectable.
			if !stop.After(start) {
				continue
			}
			effectiveStop := stop.Add(-time.Nanosecond)
			// Include every day this occurrence touches (handles crossing midnight).
			d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
			endDay := time.Date(effectiveStop.Year(), effectiveStop.Month(), effectiveStop.Day(), 0, 0, 0, 0, loc)
			for !d.After(endDay) {
				v := d.Format("2006-01-02")
				daysByValue[v] = d
				d = d.Add(24 * time.Hour)
			}
		}
	}

	availableDays := make([]time.Time, 0, len(daysByValue))
	for _, d := range daysByValue {
		availableDays = append(availableDays, d)
	}
	sort.SliceStable(availableDays, func(i, j int) bool { return availableDays[i].Before(availableDays[j]) })

	// If the user didn't select a day (or selected a day without timers), snap to the nearest day
	// that actually has an active timer occurrence. This keeps the dropdown + timeline consistent.
	if len(availableDays) > 0 {
		if _, ok := daysByValue[selectedDayStart.Format("2006-01-02")]; !ok {
			chosen := availableDays[0]
			best := chosen.Sub(selectedDayStart)
			if best < 0 {
				best = -best
			}
			for _, d := range availableDays[1:] {
				diff := d.Sub(selectedDayStart)
				if diff < 0 {
					diff = -diff
				}
				if diff < best {
					best = diff
					chosen = d
				}
			}
			selectedDayStart = chosen
			selectedDayEnd = selectedDayStart.Add(24 * time.Hour)
		}
	}

	dayOptions := make([]timerTimelineDayOption, 0, len(availableDays))
	for _, d := range availableDays {
		dayOptions = append(dayOptions, timerTimelineDayOption{
			Value: d.Format("2006-01-02"),
			Label: d.Format("Mon 2006-01-02"),
		})
	}

	resolveChannel := func(timer domain.Timer) string {
		name := nameByID[timer.ChannelID]
		if chErr == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(timer.ChannelID)); err == nil {
				if ch, ok := channelByNumber[n]; ok {
					name = ch.Name
				}
			}
		}
		if strings.TrimSpace(name) == "" {
			name = timer.ChannelID
		}
		return name
	}

	blocksByChannel := map[string][]timerTimelineBlock{}
	if len(availableDays) > 0 {
		// Only generate occurrences around the selected day; this keeps rendering fast
		// while still handling timers that cross midnight.
		blockWindowFrom := selectedDayStart.Add(-24 * time.Hour)
		blockWindowTo := selectedDayEnd.Add(24 * time.Hour)
		for _, t := range timers {
			if !t.Active {
				continue
			}
			occFrom := blockWindowFrom
			occTo := blockWindowTo
			if isWeekdayMaskHTTP(t.DaySpec) {
				if occFrom.Before(todayStart) {
					occFrom = todayStart
				}
				if occTo.After(recurringHorizonTo) {
					occTo = recurringHorizonTo
				}
				if !occFrom.Before(occTo) {
					continue
				}
			}
			for _, occ := range timerOccurrences(t, occFrom, occTo) {
				if occ.Stop.Before(selectedDayStart) || !occ.Start.Before(selectedDayEnd) {
					continue
				}

				channelName := resolveChannel(t)

				start := occ.Start.In(loc)
				stop := occ.Stop.In(loc)
				if stop.Before(start) {
					stop = start
				}
				if start.Before(selectedDayStart) {
					start = selectedDayStart
				}
				if stop.After(selectedDayEnd) {
					stop = selectedDayEnd
				}
				startMin := start.Sub(selectedDayStart).Minutes()
				stopMin := stop.Sub(selectedDayStart).Minutes()
				if stopMin <= startMin {
					// No time on this day (e.g., timer ends exactly at 00:00).
					continue
				}
				leftPct := (startMin / 1440.0) * 100.0
				widthPct := ((stopMin - startMin) / 1440.0) * 100.0
				if widthPct > 0 && widthPct < 0.25 {
					widthPct = 0.25
				}

				cls := "ok"
				if criticalIDs[t.ID] {
					cls = "critical"
				} else if collisionIDs[t.ID] {
					cls = "collision"
				}

				blocksByChannel[channelName] = append(blocksByChannel[channelName], timerTimelineBlock{
					Title:      t.Title,
					StartLabel: start.Format("15:04"),
					StopLabel:  stop.Format("15:04"),
					LeftPct:    leftPct,
					WidthPct:   widthPct,
					Class:      cls,
				})
			}
		}
	}

	rows := make([]timerTimelineRow, 0, len(blocksByChannel))
	for channelName, blocks := range blocksByChannel {
		sort.SliceStable(blocks, func(i, j int) bool {
			if blocks[i].LeftPct != blocks[j].LeftPct {
				return blocks[i].LeftPct < blocks[j].LeftPct
			}
			return blocks[i].Title < blocks[j].Title
		})
		rows = append(rows, timerTimelineRow{ChannelName: channelName, Blocks: blocks})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].ChannelName) < strings.ToLower(rows[j].ChannelName)
	})

	hours := make([]int, 24)
	for i := 0; i < 24; i++ {
		hours[i] = i
	}

	data := map[string]any{
		"Timers":              views,
		"TimelineDays":        dayOptions,
		"TimelineSelectedDay": selectedDayStart.Format("2006-01-02"),
		"TimelineHours":       hours,
		"TimelineRows":        rows,
	}

	h.renderTemplate(w, r, "timers.html", data)
}

func transponderKeyForTimer(t domain.Timer, channels []domain.Channel) string {
	chID := strings.TrimSpace(t.ChannelID)
	if looksLikeVDRChannelID(chID) {
		return transponderKeyFromChannelID(chID)
	}
	if n, err := strconv.Atoi(chID); err == nil {
		for i := range channels {
			if channels[i].Number == n {
				if looksLikeVDRChannelID(channels[i].ID) {
					return transponderKeyFromChannelID(channels[i].ID)
				}
				break
			}
		}
	}
	// Unknown channel; treat as unique so it consumes a tuner.
	return chID
}

func transponderKeyFromChannelID(channelID string) string {
	parts := strings.Split(channelID, "-")
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "-")
	}
	return channelID
}

func looksLikeVDRChannelID(s string) bool {
	s = strings.TrimSpace(s)
	return strings.Contains(s, "-") && (strings.HasPrefix(s, "S") || strings.HasPrefix(s, "C") || strings.HasPrefix(s, "T") || strings.HasPrefix(s, "A"))
}

type timerOccurrence struct {
	TimerID int
	Start   time.Time
	Stop    time.Time
	Key     string
}

func timerOverlapStates(timers []domain.Timer, dvbCards int, from, to time.Time, transponderKey func(domain.Timer) string) (collisionIDs, criticalIDs map[int]bool) {
	if dvbCards < 1 {
		dvbCards = 1
	}
	collisionIDs = map[int]bool{}
	criticalIDs = map[int]bool{}

	occs := make([]timerOccurrence, 0, len(timers))
	for _, t := range timers {
		if !t.Active {
			continue
		}
		key := transponderKey(t)
		if key == "" {
			key = strconv.Itoa(t.ID)
		}
		for _, occ := range timerOccurrences(t, from, to) {
			occ.Key = key
			occs = append(occs, occ)
		}
	}

	type endpoint struct {
		at    time.Time
		start bool
		idx   int
	}
	endpoints := make([]endpoint, 0, len(occs)*2)
	for i := range occs {
		endpoints = append(endpoints, endpoint{at: occs[i].Start, start: true, idx: i})
		endpoints = append(endpoints, endpoint{at: occs[i].Stop, start: false, idx: i})
	}

	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].at.Equal(endpoints[j].at) {
			if endpoints[i].start != endpoints[j].start {
				return !endpoints[i].start && endpoints[j].start
			}
			return endpoints[i].idx < endpoints[j].idx
		}
		return endpoints[i].at.Before(endpoints[j].at)
	})

	activeOcc := map[int]struct{}{}
	activeByKey := map[string]int{}

	for _, e := range endpoints {
		occ := occs[e.idx]
		if e.start {
			activeOcc[e.idx] = struct{}{}
			activeByKey[occ.Key]++
		} else {
			delete(activeOcc, e.idx)
			if activeByKey[occ.Key] > 1 {
				activeByKey[occ.Key]--
			} else {
				delete(activeByKey, occ.Key)
			}
		}

		// Yellow: any overlap that requires more than one transponder/tuner.
		if len(activeByKey) > 1 {
			for idx := range activeOcc {
				collisionIDs[occs[idx].TimerID] = true
			}
		}
		// Red: exceeds available DVB cards.
		if len(activeByKey) > dvbCards {
			for idx := range activeOcc {
				criticalIDs[occs[idx].TimerID] = true
			}
		}
	}

	return collisionIDs, criticalIDs
}

func timerOccurrences(t domain.Timer, from, to time.Time) []timerOccurrence {
	if from.After(to) {
		return nil
	}
	if !t.Stop.After(t.Start) || t.Start.IsZero() || t.Stop.IsZero() {
		// For recurring timers we may not have computed Start/Stop; rely on raw specs.
		// We'll handle them below if we detect a weekday mask.
	}

	// One-time timer with concrete timestamps.
	if !t.Start.IsZero() && !t.Stop.IsZero() && t.Stop.After(t.Start) && !isWeekdayMaskHTTP(t.DaySpec) {
		if t.Stop.Before(from) || !t.Start.Before(to) {
			return nil
		}
		return []timerOccurrence{{TimerID: t.ID, Start: t.Start, Stop: t.Stop}}
	}

	// Recurring timer: project onto each matching weekday in the window.
	daySpec := strings.TrimSpace(t.DaySpec)
	if !isWeekdayMaskHTTP(daySpec) {
		return nil
	}
	if t.StartMinutes < 0 || t.StopMinutes < 0 {
		return nil
	}

	loc := time.Local
	startDay := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(to.In(loc).Year(), to.In(loc).Month(), to.In(loc).Day(), 0, 0, 0, 0, loc)

	var out []timerOccurrence
	for d := startDay; d.Before(endDay); d = d.Add(24 * time.Hour) {
		if !weekdayMaskAllowsHTTP(daySpec, d.Weekday()) {
			continue
		}
		start := d.Add(time.Duration(t.StartMinutes) * time.Minute)
		stop := d.Add(time.Duration(t.StopMinutes) * time.Minute)
		if stop.Before(start) {
			stop = stop.Add(24 * time.Hour)
		}
		if stop.Before(from) || !start.Before(to) {
			continue
		}
		out = append(out, timerOccurrence{TimerID: t.ID, Start: start, Stop: stop})
	}
	return out
}

func scheduledTimerForEvent(
	ev domain.EPGEvent,
	loc *time.Location,
	occByChannelNumber map[int][]timerOccurrence,
	occByChannelID map[string][]timerOccurrence,
	numberByID map[string]int,
	idByNumber map[int]string,
	timersByID map[int]domain.Timer,
) (domain.Timer, bool) {
	// Keep scheduled detection consistent across the UI.
	// Matching by timer title is unreliable (timers vs EPG can format titles differently).
	// Instead, treat an event as scheduled when a timer on the same channel overlaps
	// most of the event window (see overlappingTimerForEvent).
	return overlappingTimerForEvent(ev, loc, occByChannelNumber, occByChannelID, numberByID, idByNumber, timersByID)
}

func isWeekdayMaskHTTP(daySpec string) bool {
	daySpec = strings.TrimSpace(daySpec)
	if len(daySpec) != 7 {
		return false
	}
	for _, r := range daySpec {
		switch r {
		case 'M', 'T', 'W', 'F', 'S', '-', '.':
			// ok
		default:
			return false
		}
	}
	return true
}

func formatClockFromMinutes(min int) string {
	if min < 0 {
		return ""
	}
	h := (min / 60) % 24
	m := min % 60
	if h < 0 {
		h += 24
	}
	if m < 0 {
		m += 60
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

func weekdayMaskFromForm(form url.Values) (string, bool) {
	letters := []rune{'M', 'T', 'W', 'T', 'F', 'S', 'S'}
	keys := []string{"wd_mon", "wd_tue", "wd_wed", "wd_thu", "wd_fri", "wd_sat", "wd_sun"}
	mask := make([]rune, 7)
	any := false
	for i := range mask {
		if strings.TrimSpace(form.Get(keys[i])) != "" {
			mask[i] = letters[i]
			any = true
		} else {
			mask[i] = '-'
		}
	}
	return string(mask), any
}

func weekdayMaskAllowsHTTP(daySpec string, wd time.Weekday) bool {
	idx := 0
	switch wd {
	case time.Monday:
		idx = 0
	case time.Tuesday:
		idx = 1
	case time.Wednesday:
		idx = 2
	case time.Thursday:
		idx = 3
	case time.Friday:
		idx = 4
	case time.Saturday:
		idx = 5
	case time.Sunday:
		idx = 6
	}
	if idx < 0 || idx >= len(daySpec) {
		return false
	}
	c := daySpec[idx]
	return c != '-' && c != '.'
}

type timerFormModel struct {
	ID        int
	Active    bool
	ChannelID string
	Day       string
	DayMode   string
	WDMon     bool
	WDTue     bool
	WDWed     bool
	WDThu     bool
	WDFri     bool
	WDSat     bool
	WDSun     bool
	Start     string
	Stop      string
	Priority  int
	Lifetime  int
	Title     string
	Aux       string
}

func (h *Handler) timerNewFormModel(now time.Time, channels []domain.Channel) (timerFormModel, string) {
	selectedChannel := ""
	if len(channels) > 0 {
		selectedChannel = channels[0].ID
	}

	localNow := now.In(time.Local)
	day := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)

	model := timerFormModel{
		ID:        0,
		Active:    true,
		ChannelID: selectedChannel,
		Day:       day.Format("2006-01-02"),
		DayMode:   "single",
		Start:     localNow.Format("15:04"),
		Stop:      "00:00",
		Priority:  99,
		Lifetime:  99,
		Title:     "",
		Aux:       "",
	}
	return model, selectedChannel
}

// TimerNew shows a form for creating a new manual timer.
func (h *Handler) TimerNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channels, err := h.epgService.GetChannels(r.Context())
	if err != nil {
		channels = []domain.Channel{}
	}

	model, selectedChannel := h.timerNewFormModel(time.Now(), channels)

	// Optional prefill (used by EPG search "Record" links).
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("channel")); v != "" {
		model.ChannelID = v
		selectedChannel = v
	}
	if v := strings.TrimSpace(q.Get("day")); v != "" {
		if _, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			model.Day = v
		}
	}
	if v := strings.TrimSpace(q.Get("start")); v != "" {
		if _, err := time.Parse("15:04", v); err == nil {
			model.Start = v
		}
	}
	if v := strings.TrimSpace(q.Get("stop")); v != "" {
		if _, err := time.Parse("15:04", v); err == nil {
			model.Stop = v
		}
	}
	if v := strings.TrimSpace(q.Get("title")); v != "" {
		model.Title = v
	}
	data := map[string]any{
		"PageTitle":       "New Timer - VDRAdmin-go",
		"Heading":         "New Timer",
		"FormAction":      "/timers/new",
		"Timer":           model,
		"SelectedChannel": selectedChannel,
		"Channels":        channels,
	}
	h.renderTemplate(w, r, "timer_edit.html", data)
}

// TimerEdit shows a form for editing an existing timer.
func (h *Handler) TimerEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timerID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if timerID <= 0 {
		http.Error(w, "Invalid timer ID", http.StatusBadRequest)
		return
	}

	timers, err := h.timerService.GetAllTimers(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	var found *domain.Timer
	for i := range timers {
		if timers[i].ID == timerID {
			found = &timers[i]
			break
		}
	}
	if found == nil {
		h.handleError(w, r, domain.ErrNotFound)
		return
	}

	channels, chErr := h.epgService.GetChannels(r.Context())
	if chErr != nil {
		channels = []domain.Channel{}
	}

	model := h.timerToFormModel(*found)
	selectedChannel := ""
	for i := range channels {
		if channels[i].ID != "" && channels[i].ID == found.ChannelID {
			selectedChannel = channels[i].ID
			break
		}
	}
	if selectedChannel == "" {
		if n, err := strconv.Atoi(found.ChannelID); err == nil {
			for i := range channels {
				if channels[i].Number == n {
					selectedChannel = channels[i].ID
					break
				}
			}
		}
	}

	// If the timer references an unknown channel, append a single fallback option at the end.
	// This avoids duplicates and keeps known channels in their original order.
	if selectedChannel == "" && found.ChannelID != "" {
		channels = append(channels, domain.Channel{ID: found.ChannelID, Name: found.ChannelID})
		selectedChannel = found.ChannelID
	}

	data := map[string]any{
		"PageTitle":       "Edit Timer - VDRAdmin-go",
		"Heading":         "Edit Timer",
		"FormAction":      "/timers/update",
		"Timer":           model,
		"SelectedChannel": selectedChannel,
		"Channels":        channels,
	}
	h.renderTemplate(w, r, "timer_edit.html", data)
}

func (h *Handler) timerFromCreateForm(r *http.Request) (domain.Timer, error) {
	form := r.Form

	channelID := strings.TrimSpace(form.Get("channel"))
	if channelID == "" {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	dayMode := strings.TrimSpace(form.Get("day_mode"))
	if dayMode == "" {
		dayMode = "single"
	}

	startStr := strings.TrimSpace(form.Get("start"))
	stopStr := strings.TrimSpace(form.Get("stop"))
	startClock, err1 := time.Parse("15:04", startStr)
	stopClock, err2 := time.Parse("15:04", stopStr)
	if err1 != nil || err2 != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}
	startMinutes := startClock.Hour()*60 + startClock.Minute()
	stopMinutes := stopClock.Hour()*60 + stopClock.Minute()

	day := time.Time{}
	start := time.Time{}
	stop := time.Time{}
	daySpec := ""
	if dayMode == "weekly" {
		mask, any := weekdayMaskFromForm(form)
		if !any {
			return domain.Timer{}, domain.ErrInvalidInput
		}
		daySpec = mask
	} else {
		dayStr := strings.TrimSpace(form.Get("day"))
		parsedDay, err := time.ParseInLocation("2006-01-02", dayStr, time.Local)
		if err != nil {
			return domain.Timer{}, domain.ErrInvalidInput
		}
		day = parsedDay
		start = time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
		stop = time.Date(day.Year(), day.Month(), day.Day(), stopClock.Hour(), stopClock.Minute(), 0, 0, time.Local)
		if stop.Before(start) {
			stop = stop.Add(24 * time.Hour)
		}
		// Ensure this is treated as a one-time timer.
		daySpec = ""
		startMinutes = -1
		stopMinutes = -1
	}

	priority, err := strconv.Atoi(strings.TrimSpace(form.Get("priority")))
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}
	lifetime, err := strconv.Atoi(strings.TrimSpace(form.Get("lifetime")))
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	active := strings.TrimSpace(form.Get("active")) != "0"
	title := strings.TrimSpace(form.Get("title"))
	aux := form.Get("aux")

	return domain.Timer{
		ID:        0,
		Active:    active,
		ChannelID: channelID,
		Day:       day,
		Start:     start,
		Stop:      stop,
		DaySpec:   daySpec,
		// For repeating timers, these clocks are required.
		StartMinutes: startMinutes,
		StopMinutes:  stopMinutes,
		Priority:     priority,
		Lifetime:     lifetime,
		Title:        title,
		Aux:          aux,
	}, nil
}

// TimerCreateManual creates a new timer from the manual form.
func (h *Handler) TimerCreateManual(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handleError(w, r, err)
		return
	}

	timer, err := h.timerFromCreateForm(r)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	if err := h.timerService.CreateTimer(r.Context(), &timer); err != nil {
		h.handleError(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/timers")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/timers", http.StatusSeeOther)
}

// TimerUpdate persists edits to an existing timer.
func (h *Handler) TimerUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handleError(w, r, err)
		return
	}

	timer, err := h.timerFromForm(r)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	if err := h.timerService.UpdateTimer(r.Context(), &timer); err != nil {
		h.handleError(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/timers")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/timers", http.StatusSeeOther)
}

func (h *Handler) timerToFormModel(t domain.Timer) timerFormModel {
	day := t.Day
	if day.IsZero() {
		day = t.Start
	}

	dayStr := ""
	if !day.IsZero() {
		dayStr = day.In(time.Local).Format("2006-01-02")
	}

	dayMode := "single"
	daySpec := strings.TrimSpace(t.DaySpec)
	if isWeekdayMaskHTTP(daySpec) {
		dayMode = "weekly"
	}

	startStr := ""
	stopStr := ""
	if dayMode == "weekly" {
		if t.StartMinutes >= 0 {
			startStr = formatClockFromMinutes(t.StartMinutes)
		}
		if t.StopMinutes >= 0 {
			stopStr = formatClockFromMinutes(t.StopMinutes)
		}
	}
	if startStr == "" && !t.Start.IsZero() {
		startStr = t.Start.In(time.Local).Format("15:04")
	}
	if stopStr == "" && !t.Stop.IsZero() {
		stopStr = t.Stop.In(time.Local).Format("15:04")
	}

	wdMon := false
	wdTue := false
	wdWed := false
	wdThu := false
	wdFri := false
	wdSat := false
	wdSun := false
	if dayMode == "weekly" && len(daySpec) == 7 {
		wdMon = daySpec[0] != '-' && daySpec[0] != '.'
		wdTue = daySpec[1] != '-' && daySpec[1] != '.'
		wdWed = daySpec[2] != '-' && daySpec[2] != '.'
		wdThu = daySpec[3] != '-' && daySpec[3] != '.'
		wdFri = daySpec[4] != '-' && daySpec[4] != '.'
		wdSat = daySpec[5] != '-' && daySpec[5] != '.'
		wdSun = daySpec[6] != '-' && daySpec[6] != '.'
	}

	return timerFormModel{
		ID:        t.ID,
		Active:    t.Active,
		ChannelID: t.ChannelID,
		Day:       dayStr,
		DayMode:   dayMode,
		WDMon:     wdMon,
		WDTue:     wdTue,
		WDWed:     wdWed,
		WDThu:     wdThu,
		WDFri:     wdFri,
		WDSat:     wdSat,
		WDSun:     wdSun,
		Start:     startStr,
		Stop:      stopStr,
		Priority:  t.Priority,
		Lifetime:  t.Lifetime,
		Title:     t.Title,
		Aux:       t.Aux,
	}
}

func (h *Handler) timerFromForm(r *http.Request) (domain.Timer, error) {
	form := r.Form

	id, _ := strconv.Atoi(form.Get("id"))
	if id <= 0 {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	channelID := strings.TrimSpace(form.Get("channel"))
	if channelID == "" {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	dayMode := strings.TrimSpace(form.Get("day_mode"))
	if dayMode == "" {
		dayMode = "single"
	}

	startStr := strings.TrimSpace(form.Get("start"))
	stopStr := strings.TrimSpace(form.Get("stop"))
	startClock, err1 := time.Parse("15:04", startStr)
	stopClock, err2 := time.Parse("15:04", stopStr)
	if err1 != nil || err2 != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}
	startMinutes := startClock.Hour()*60 + startClock.Minute()
	stopMinutes := stopClock.Hour()*60 + stopClock.Minute()

	day := time.Time{}
	start := time.Time{}
	stop := time.Time{}
	daySpec := ""
	if dayMode == "weekly" {
		mask, any := weekdayMaskFromForm(form)
		if !any {
			return domain.Timer{}, domain.ErrInvalidInput
		}
		daySpec = mask
	} else {
		dayStr := strings.TrimSpace(form.Get("day"))
		parsedDay, err := time.ParseInLocation("2006-01-02", dayStr, time.Local)
		if err != nil {
			return domain.Timer{}, domain.ErrInvalidInput
		}
		day = parsedDay
		start = time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
		stop = time.Date(day.Year(), day.Month(), day.Day(), stopClock.Hour(), stopClock.Minute(), 0, 0, time.Local)
		if stop.Before(start) {
			stop = stop.Add(24 * time.Hour)
		}
		daySpec = ""
		startMinutes = -1
		stopMinutes = -1
	}

	priority, err := strconv.Atoi(strings.TrimSpace(form.Get("priority")))
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}
	lifetime, err := strconv.Atoi(strings.TrimSpace(form.Get("lifetime")))
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	title := strings.TrimSpace(form.Get("title"))
	if title == "" {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	active := strings.TrimSpace(form.Get("active")) != "0"
	aux := form.Get("aux")

	return domain.Timer{
		ID:           id,
		Active:       active,
		ChannelID:    channelID,
		Day:          day,
		Start:        start,
		Stop:         stop,
		DaySpec:      daySpec,
		StartMinutes: startMinutes,
		StopMinutes:  stopMinutes,
		Priority:     priority,
		Lifetime:     lifetime,
		Title:        title,
		Aux:          aux,
	}, nil
}

// TimerCreate creates a new timer
func (h *Handler) TimerCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		h.handleError(w, r, err)
		return
	}

	eventID, _ := strconv.Atoi(r.FormValue("event_id"))
	channelID := r.FormValue("channel")
	if eventID > 0 {
		// Create from EPG
		var events []domain.EPGEvent
		var err error
		if channelID != "" {
			events, err = h.epgService.GetEPG(r.Context(), channelID, time.Time{})
		} else {
			events, err = h.epgService.GetEPG(r.Context(), "", time.Time{})
		}
		if err != nil {
			h.handleError(w, r, err)
			return
		}

		for _, event := range events {
			if event.EventID != eventID {
				continue
			}
			if channelID != "" && event.ChannelID != channelID {
				continue
			}

			priority, lifetime, marginStart, marginEnd := h.timerDefaults()
			err := h.timerService.CreateTimerFromEPG(r.Context(), event, priority, lifetime, marginStart, marginEnd)
			if err != nil {
				h.handleError(w, r, err)
				return
			}

			// Support both HTMX requests and normal browser form posts.
			if r.Header.Get("HX-Request") != "" {
				w.Header().Set("HX-Redirect", "/timers")
				w.WriteHeader(http.StatusOK)
			} else {
				http.Redirect(w, r, "/timers", http.StatusSeeOther)
			}
			return
		}
	}

	http.Error(w, "Event not found", http.StatusNotFound)
}

func (h *Handler) timerDefaults() (priority, lifetime, marginStart, marginEnd int) {
	// Keep behavior stable even if config isn't wired (e.g., in tests or early boot).
	priority = 50
	lifetime = 99
	marginStart = 2
	marginEnd = 10

	if h.cfg == nil {
		return priority, lifetime, marginStart, marginEnd
	}

	if h.cfg.Timer.DefaultPriority >= 0 {
		priority = h.cfg.Timer.DefaultPriority
	}
	if h.cfg.Timer.DefaultLifetime >= 0 {
		lifetime = h.cfg.Timer.DefaultLifetime
	}
	if h.cfg.Timer.DefaultMarginStart >= 0 {
		marginStart = h.cfg.Timer.DefaultMarginStart
	}
	if h.cfg.Timer.DefaultMarginEnd >= 0 {
		marginEnd = h.cfg.Timer.DefaultMarginEnd
	}

	return priority, lifetime, marginStart, marginEnd
}

// TimerDelete deletes a timer
func (h *Handler) TimerDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timerID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if timerID == 0 {
		http.Error(w, "Invalid timer ID", http.StatusBadRequest)
		return
	}

	if err := h.timerService.DeleteTimer(r.Context(), timerID); err != nil {
		h.handleError(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/timers")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// TimerToggle toggles a timer's active state
func (h *Handler) TimerToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	timerID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if timerID == 0 {
		http.Error(w, "Invalid timer ID", http.StatusBadRequest)
		return
	}

	if err := h.timerService.ToggleTimer(r.Context(), timerID); err != nil {
		h.handleError(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/timers")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RecordingList shows all recordings
func (h *Handler) RecordingList(w http.ResponseWriter, r *http.Request) {
	recordings, err := h.recordingService.GetAllRecordings(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	includeSubtitle := isTruthy(r.URL.Query().Get("in_subtitle"))
	includePath := isTruthy(r.URL.Query().Get("in_path"))
	recordings = filterRecordings(recordings, q, includeSubtitle, includePath)

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "date"
	}
	recordings = h.recordingService.SortRecordings(recordings, sortBy)

	data := map[string]any{
		"Recordings": recordings,
		"Sort":       sortBy,
		"Query":      q,
		"InSubtitle": includeSubtitle,
		"InPath":     includePath,
	}
	if role, _ := r.Context().Value("role").(string); role == "admin" {
		data["ActiveArchiveJobs"] = h.archiveJobs.ActiveJobIDsByRecording()
	}

	h.renderTemplate(w, r, "recordings.html", data)
}

// RecordingRefresh invalidates the recordings cache and returns a fresh list.
// It is intended for cases where recordings are changed out-of-band (e.g. deleted on disk).
func (h *Handler) RecordingRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.recordingService == nil {
		http.Error(w, "Recording service not available", http.StatusInternalServerError)
		return
	}

	_ = r.ParseForm()
	// Prefer form value (htmx), fall back to query.
	sortBy := strings.TrimSpace(r.FormValue("sort"))
	if sortBy == "" {
		sortBy = strings.TrimSpace(r.URL.Query().Get("sort"))
	}
	if sortBy == "" {
		sortBy = "date"
	}

	q := strings.TrimSpace(r.FormValue("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	includeSubtitle := isTruthy(r.FormValue("in_subtitle"))
	if !includeSubtitle {
		includeSubtitle = isTruthy(r.URL.Query().Get("in_subtitle"))
	}
	includePath := isTruthy(r.FormValue("in_path"))
	if !includePath {
		includePath = isTruthy(r.URL.Query().Get("in_path"))
	}

	h.recordingService.InvalidateCache()

	recordings, err := h.recordingService.GetAllRecordings(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	recordings = filterRecordings(recordings, q, includeSubtitle, includePath)
	recordings = h.recordingService.SortRecordings(recordings, sortBy)

	data := map[string]any{
		"Recordings": recordings,
		"Sort":       sortBy,
		"Query":      q,
		"InSubtitle": includeSubtitle,
		"InPath":     includePath,
	}
	if role, _ := r.Context().Value("role").(string); role == "admin" {
		data["ActiveArchiveJobs"] = h.archiveJobs.ActiveJobIDsByRecording()
	}

	// For non-HTMX browsers, behave like a standard action.
	if r.Header.Get("HX-Request") == "" {
		params := url.Values{}
		params.Set("sort", sortBy)
		if strings.TrimSpace(q) != "" {
			params.Set("q", q)
		}
		if includeSubtitle {
			params.Set("in_subtitle", "1")
		}
		if includePath {
			params.Set("in_path", "1")
		}
		http.Redirect(w, r, "/recordings?"+params.Encode(), http.StatusSeeOther)
		return
	}

	h.renderTemplate(w, r, "recordings.html", data)
}

func filterRecordings(recordings []domain.Recording, query string, includeSubtitle bool, includePath bool) []domain.Recording {
	query = strings.TrimSpace(query)
	if query == "" {
		return recordings
	}
	if utf8.RuneCountInString(query) < 3 {
		return recordings
	}

	q := strings.ToLower(query)
	filtered := make([]domain.Recording, 0, len(recordings))
	for _, rec := range recordings {
		haystackParts := []string{rec.Title}
		if includeSubtitle {
			haystackParts = append(haystackParts, rec.Subtitle)
		}
		if includePath {
			haystackParts = append(haystackParts, rec.Path)
		}
		haystack := strings.ToLower(strings.Join(haystackParts, "\n"))
		if strings.Contains(haystack, q) {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func isTruthy(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// RecordingDelete deletes a recording
func (h *Handler) RecordingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	if err := h.recordingService.DeleteRecording(r.Context(), path); err != nil {
		h.handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// RecordingArchivePrepare shows a preview form for archiving a recording.
// MVP: preview-only (directory naming + target paths), no ffmpeg execution yet.
func (h *Handler) RecordingArchivePrepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	if h.vdrClient == nil {
		http.Error(w, "VDR client not available", http.StatusInternalServerError)
		return
	}

	recID := strings.TrimSpace(r.URL.Query().Get("path"))
	if recID == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	if jobID, ok := h.archiveJobs.ActiveJobIDForRecording(recID); ok {
		http.Redirect(w, r, "/recordings/archive/job?id="+url.QueryEscape(jobID), http.StatusFound)
		return
	}

	recDir, err := h.vdrClient.GetRecordingDir(r.Context(), recID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if strings.TrimSpace(recDir) == "" {
		h.renderTemplate(w, r, "recording_archive.html", map[string]any{
			"Error":        "Could not resolve recording directory via VDR.",
			"RecordingID":  recID,
			"RecordingDir": "",
		})
		return
	}

	infoPath := filepath.Join(recDir, "info")
	infoBytes, err := os.ReadFile(infoPath)
	if err != nil {
		h.renderTemplate(w, r, "recording_archive.html", map[string]any{
			"Error":        fmt.Sprintf("Failed to read info file: %v", err),
			"RecordingID":  recID,
			"RecordingDir": recDir,
		})
		return
	}

	parsed, err := archive.ParseVDRInfo(strings.NewReader(string(infoBytes)))
	if err != nil {
		h.renderTemplate(w, r, "recording_archive.html", map[string]any{
			"Error":        fmt.Sprintf("Failed to parse info file: %v", err),
			"RecordingID":  recID,
			"RecordingDir": recDir,
		})
		return
	}

	// Allow user overrides via query params.
	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" {
		title = parsed.Title
	}
	episode := strings.TrimSpace(r.URL.Query().Get("episode"))
	if episode == "" {
		episode = parsed.Episode
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "mp4" {
		format = "mkv"
	}

	profiles := h.archiveProfilesFromConfig(h.cfg)
	sort.SliceStable(profiles, func(i, j int) bool {
		ai := strings.ToLower(strings.TrimSpace(profiles[i].Name))
		aj := strings.ToLower(strings.TrimSpace(profiles[j].Name))
		if ai != aj {
			return ai < aj
		}
		return strings.ToLower(strings.TrimSpace(profiles[i].ID)) < strings.ToLower(strings.TrimSpace(profiles[j].ID))
	})
	selectedID := strings.TrimSpace(r.URL.Query().Get("profile"))
	if selectedID == "" {
		selectedID = h.defaultProfileIDForKind(profiles, parsed.Kind)
	}

	const profileNoneID = "none"
	var preview archive.Preview
	var perr error
	if selectedID != profileNoneID {
		selected, ok := archive.FindProfile(profiles, selectedID)
		if !ok {
			selectedID = h.defaultProfileIDForKind(profiles, parsed.Kind)
			selected, _ = archive.FindProfile(profiles, selectedID)
		}
		preview, perr = archive.BuildPreview(selected, title, episode, format)
	}
	var warn string
	if strings.TrimSpace(h.cfg.Archive.BaseDir) == "" {
		warn = "archive.base_dir is not set yet. Configure it in Configurations → Archive to get correct absolute target paths."
	}

	data := map[string]any{
		"RecordingID":       recID,
		"RecordingDir":      recDir,
		"DetectedKind":      string(parsed.Kind),
		"Title":             title,
		"Episode":           episode,
		"Profiles":          profiles,
		"SelectedProfileID": selectedID,
		"Format":            format,
		"ArchiveWarning":    warn,
	}
	if perr != nil {
		data["Error"] = perr.Error()
	} else {
		if selectedID != profileNoneID {
			data["Preview"] = preview
			if preview.VideoPath != "" {
				if _, err := os.Stat(preview.VideoPath); err == nil {
					data["OutputExists"] = true
					data["OutputExistsPath"] = preview.VideoPath
				}
			}
		}
	}

	h.renderTemplate(w, r, "recording_archive.html", data)
}

// RecordingArchiveStart starts a background archive job and redirects to the job page.
func (h *Handler) RecordingArchiveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	if h.vdrClient == nil {
		http.Error(w, "VDR client not available", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	recID := strings.TrimSpace(r.FormValue("path"))
	if recID == "" {
		recID = strings.TrimSpace(r.FormValue("recording_id"))
	}
	if recID == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	if jobID, ok := h.archiveJobs.ActiveJobIDForRecording(recID); ok {
		redirect := "/recordings/archive/job?id=" + url.QueryEscape(jobID)
		if r.Header.Get("HX-Request") != "" {
			w.Header().Set("HX-Redirect", redirect)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	episode := strings.TrimSpace(r.FormValue("episode"))
	profileID := strings.TrimSpace(r.FormValue("profile"))
	format := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	if format != "mp4" {
		format = "mkv"
	}
	// Optional user overrides from the Preview section.
	oTargetDir := strings.TrimSpace(r.FormValue("target_dir"))
	oVideoPath := strings.TrimSpace(r.FormValue("video_path"))
	oInfoDstPath := strings.TrimSpace(r.FormValue("info_dst_path"))

	recDir, err := h.vdrClient.GetRecordingDir(r.Context(), recID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if strings.TrimSpace(recDir) == "" {
		http.Error(w, "Could not resolve recording directory", http.StatusBadRequest)
		return
	}

	infoPath := filepath.Join(recDir, "info")
	infoBytes, err := os.ReadFile(infoPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read info file: %v", err), http.StatusBadRequest)
		return
	}
	parsed, err := archive.ParseVDRInfo(strings.NewReader(string(infoBytes)))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse info file: %v", err), http.StatusBadRequest)
		return
	}
	if title == "" {
		title = parsed.Title
	}
	if episode == "" {
		episode = parsed.Episode
	}

	profiles := h.archiveProfilesFromConfig(h.cfg)
	sort.SliceStable(profiles, func(i, j int) bool {
		ai := strings.ToLower(strings.TrimSpace(profiles[i].Name))
		aj := strings.ToLower(strings.TrimSpace(profiles[j].Name))
		if ai != aj {
			return ai < aj
		}
		return strings.ToLower(strings.TrimSpace(profiles[i].ID)) < strings.ToLower(strings.TrimSpace(profiles[j].ID))
	})
	const profileNoneID = "none"

	ffArgs := archive.SplitArgs(h.cfg.Archive.FFMpegArgs)
	var plan archive.Plan
	var planErr error

	if strings.TrimSpace(profileID) == profileNoneID {
		// For "None", only TargetDir is editable; output paths are derived from TargetDir + format.
		custom := archive.Preview{TargetDir: oTargetDir}
		plan, planErr = archive.BuildPlanWithPreview(recID, recDir, infoPath, archive.ArchiveProfile{ID: profileNoneID, Name: "None", Kind: parsed.Kind}, custom, format, ffArgs)
	} else {
		if profileID == "" {
			profileID = h.defaultProfileIDForKind(profiles, parsed.Kind)
		}
		selected, ok := archive.FindProfile(profiles, profileID)
		if !ok {
			selected, _ = archive.FindProfile(profiles, h.defaultProfileIDForKind(profiles, parsed.Kind))
		}
		plan, planErr = archive.BuildPlan(recID, recDir, infoPath, selected, title, episode, format, ffArgs)
	}
	if planErr != nil {
		h.renderTemplate(w, r, "recording_archive.html", map[string]any{
			"Error":             planErr.Error(),
			"RecordingID":       recID,
			"RecordingDir":      recDir,
			"DetectedKind":      string(parsed.Kind),
			"Title":             title,
			"Episode":           episode,
			"Profiles":          profiles,
			"SelectedProfileID": profileID,
			"Format":            format,
			"Preview": &archive.Preview{
				TargetDir:   oTargetDir,
				VideoPath:   oVideoPath,
				InfoDstPath: oInfoDstPath,
			},
		})
		return
	}
	// Apply overrides after the plan is built so Profile changes won't clobber user edits.
	if strings.TrimSpace(profileID) != profileNoneID {
		if oTargetDir != "" {
			plan.Preview.TargetDir = oTargetDir
			// Only auto-derive paths from target dir when the user didn't override them.
			if oVideoPath == "" {
				plan.Preview.VideoPath = filepath.Join(oTargetDir, "video."+format)
			}
			if oInfoDstPath == "" {
				plan.Preview.InfoDstPath = filepath.Join(oTargetDir, "video.info")
			}
		}
		if oVideoPath != "" {
			plan.Preview.VideoPath = oVideoPath
		}
		if oInfoDstPath != "" {
			plan.Preview.InfoDstPath = oInfoDstPath
		}
	}
	if plan.Preview.VideoPath != "" {
		if _, err := os.Stat(plan.Preview.VideoPath); err == nil {
			h.renderTemplate(w, r, "recording_archive.html", map[string]any{
				"Error":             fmt.Sprintf("Output already exists: %s", plan.Preview.VideoPath),
				"RecordingID":       recID,
				"RecordingDir":      recDir,
				"DetectedKind":      string(parsed.Kind),
				"Title":             title,
				"Episode":           episode,
				"Profiles":          profiles,
				"SelectedProfileID": profileID,
				"Preview":           plan.Preview,
				"OutputExists":      true,
				"OutputExistsPath":  plan.Preview.VideoPath,
			})
			return
		}
	}

	jobID, err := h.archiveJobs.Start(context.Background(), plan, h.instanceID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.logger.Info("archive job started",
		slog.String("job_id", jobID),
		slog.String("instance_id", h.instanceID),
		slog.String("target_dir", plan.Preview.TargetDir),
		slog.String("video_path", plan.Preview.VideoPath),
	)

	redirect := "/recordings/archive/job?id=" + url.QueryEscape(jobID)
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", redirect)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

// RecordingArchivePreview returns suggested preview values for a given profile/title/episode.
// The client applies these suggestions only to fields the user hasn't edited.
func (h *Handler) RecordingArchivePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}

	title := strings.TrimSpace(r.URL.Query().Get("title"))
	episode := strings.TrimSpace(r.URL.Query().Get("episode"))
	profileID := strings.TrimSpace(r.URL.Query().Get("profile"))
	currentVideoPath := strings.TrimSpace(r.URL.Query().Get("video_path"))
	targetDir := strings.TrimSpace(r.URL.Query().Get("target_dir"))
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "mp4" {
		format = "mkv"
	}

	const profileNoneID = "none"
	var preview archive.Preview
	if profileID == profileNoneID {
		p, _ := archive.NormalizePreview(archive.Preview{TargetDir: targetDir}, format)
		preview = p
	} else {
		profiles := h.archiveProfilesFromConfig(h.cfg)
		selected, ok := archive.FindProfile(profiles, profileID)
		if !ok {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unknown profile"})
			return
		}
		p, err := archive.BuildPreview(selected, title, episode, format)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		preview = p
	}

	// Output existence warning should reflect the current effective path, not necessarily the suggested one.
	checkPath := preview.VideoPath
	if currentVideoPath != "" {
		checkPath = currentVideoPath
	}
	outputExists := false
	if checkPath != "" {
		if _, statErr := os.Stat(checkPath); statErr == nil {
			outputExists = true
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title_slug":         preview.TitleSlug,
		"episode_slug":       preview.EpisodeSlug,
		"target_dir":         preview.TargetDir,
		"video_path":         preview.VideoPath,
		"info_dst_path":      preview.InfoDstPath,
		"output_exists":      outputExists,
		"output_exists_path": checkPath,
	})
}

// RecordingArchiveJobs renders a simple overview of recent archive jobs.
func (h *Handler) RecordingArchiveJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobs := h.archiveJobs.List()
	h.renderTemplate(w, r, "recording_archive_jobs.html", map[string]any{
		"Jobs": jobs,
	})
}

// RecordingArchiveJob renders the job progress page.
func (h *Handler) RecordingArchiveJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := normalizeArchiveJobID(r.URL.Query().Get("id"))
	if jobID == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}
	snap, ok := h.archiveJobs.Get(jobID)
	if !ok {
		http.Error(w, "Unknown job", http.StatusNotFound)
		return
	}
	recordingTitle := "—"
	if h.recordingService != nil {
		if title, ok, err := h.findRecordingTitleByPath(r.Context(), snap.RecordingID); err == nil && ok {
			recordingTitle = title
		}
	}
	infoCopyText, infoCopyErr := archiveJobInfoCopyText(snap)
	outputReady := archiveJobOutputReady(snap)
	data := map[string]any{
		"Job":              snap,
		"JobID":            jobID,
		"InstanceID":       h.instanceID,
		"PID":              h.pid,
		"RecordingTitle":   recordingTitle,
		"InfoCopyText":     infoCopyText,
		"InfoCopyError":    infoCopyErr,
		"OutputVideoReady": outputReady,
		"OutputVideoURL":   "/recordings/archive/job/output?id=" + url.QueryEscape(jobID),
	}
	h.renderTemplate(w, r, "recording_archive_job.html", data)
}

func (h *Handler) findRecordingTitleByPath(ctx context.Context, recordingPath string) (string, bool, error) {
	recordingPath = strings.TrimSpace(recordingPath)
	if recordingPath == "" {
		return "", false, nil
	}
	recs, err := h.recordingService.GetAllRecordings(ctx)
	if err != nil {
		return "", false, err
	}
	for _, rec := range recs {
		if strings.TrimSpace(rec.Path) != recordingPath {
			continue
		}
		title := strings.TrimSpace(rec.Title)
		sub := strings.TrimSpace(rec.Subtitle)
		if title == "" {
			return "", false, nil
		}
		if sub != "" {
			return title + " — " + sub, true, nil
		}
		return title, true, nil
	}
	return "", false, nil
}

// RecordingArchiveJobPoll returns a small JSON payload for progress and incremental logs.
// This avoids flicker caused by swapping large HTML fragments and allows appending logs
// without resetting the scroll position.
func (h *Handler) RecordingArchiveJobPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "method not allowed", "instance_id": h.instanceID, "pid": h.pid})
		return
	}
	jobID := normalizeArchiveJobID(r.URL.Query().Get("id"))
	if jobID == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing job id", "instance_id": h.instanceID, "pid": h.pid})
		return
	}
	from := 0
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			from = n
		}
	}

	snap, newLines, next, ok := h.archiveJobs.Poll(jobID, from)
	if !ok {
		jobCount := h.archiveJobs.Count()
		h.logger.Warn("archive job poll: unknown job",
			slog.String("job_id", jobID),
			slog.String("instance_id", h.instanceID),
			slog.Int("job_count", jobCount),
		)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unknown job", "id": jobID, "instance_id": h.instanceID, "pid": h.pid, "job_count": jobCount})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	infoCopyText, infoCopyErr := archiveJobInfoCopyText(snap)
	outputReady := archiveJobOutputReady(snap)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"instance_id":     h.instanceID,
		"pid":             h.pid,
		"id":              snap.ID,
		"status":          snap.Status,
		"error":           snap.Error,
		"info_copy_text":  infoCopyText,
		"info_copy_error": infoCopyErr,
		"video_path":      snap.Preview.VideoPath,
		"output_ready":    outputReady,
		"output_url":      "/recordings/archive/job/output?id=" + url.QueryEscape(snap.ID),
		"log_next":        next,
		"log_lines":       newLines,
		"progress": map[string]any{
			"known_duration": snap.Progress.KnownDuration,
			"percent":        snap.Progress.Percent,
			"out_time_ms":    snap.Progress.OutTimeMS,
			"speed":          snap.Progress.Speed,
		},
	})
}

// RecordingArchiveJobOutput serves the finished archive output video file.
func (h *Handler) RecordingArchiveJobOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := normalizeArchiveJobID(r.URL.Query().Get("id"))
	if jobID == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}
	snap, ok := h.archiveJobs.Get(jobID)
	if !ok {
		http.Error(w, "Unknown job", http.StatusNotFound)
		return
	}
	videoPath := strings.TrimSpace(snap.Preview.VideoPath)
	if videoPath == "" {
		http.Error(w, "Missing output video path", http.StatusNotFound)
		return
	}
	// Safety: only allow serving files inside the job's TargetDir.
	targetDir := filepath.Clean(strings.TrimSpace(snap.Preview.TargetDir))
	cleanVideo := filepath.Clean(videoPath)
	if targetDir == "." || targetDir == "" {
		http.Error(w, "Invalid target dir", http.StatusBadRequest)
		return
	}
	rel, err := filepath.Rel(targetDir, cleanVideo)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		http.Error(w, "Refusing to serve file outside target dir", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(cleanVideo); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Output video not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to stat output video", http.StatusInternalServerError)
		return
	}

	// Prefer an inline content disposition so the browser can play it;
	// users can still open/save it in their video player.
	filename := filepath.Base(cleanVideo)
	if filename != "" {
		disp := "inline"
		switch strings.ToLower(filepath.Ext(filename)) {
		case ".mkv":
			// Most browsers don't reliably play MKV inline.
			disp = "attachment"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": filename}))
	}
	ext := strings.ToLower(filepath.Ext(cleanVideo))
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if ext == ".mkv" {
		w.Header().Set("Content-Type", "video/x-matroska")
	}
	http.ServeFile(w, r, cleanVideo)
}

func archiveJobInfoCopyText(snap archive.JobSnapshot) (text string, errMsg string) {
	// Only attempt once the job is no longer running.
	if snap.Status == archive.JobQueued || snap.Status == archive.JobRunning {
		return "", ""
	}
	path := strings.TrimSpace(snap.Preview.InfoDstPath)
	if path == "" {
		return "", ""
	}

	const maxBytes = int64(256 * 1024)
	b, truncated, err := readFileLimited(path, maxBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "info copy file not found"
		}
		return "", err.Error()
	}
	if len(b) == 0 {
		return "", ""
	}
	out := string(b)
	out = strings.ToValidUTF8(out, "�")
	if truncated {
		out += fmt.Sprintf("\n\n…(truncated to %d bytes)…", maxBytes)
	}
	return out, ""
}

func archiveJobOutputReady(snap archive.JobSnapshot) bool {
	// Only claim readiness once the job is no longer running.
	if snap.Status == archive.JobQueued || snap.Status == archive.JobRunning {
		return false
	}
	path := strings.TrimSpace(snap.Preview.VideoPath)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func readFileLimited(path string, maxBytes int64) (b []byte, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	if maxBytes < 0 {
		maxBytes = 0
	}
	// Read up to maxBytes+1 so we can detect truncation.
	buf, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(buf)) > maxBytes {
		return buf[:maxBytes], true, nil
	}
	return buf, false, nil
}

// RecordingArchiveJobStatus returns an HTML fragment with current progress/logs.
func (h *Handler) RecordingArchiveJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := normalizeArchiveJobID(r.URL.Query().Get("id"))
	if jobID == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}
	snap, ok := h.archiveJobs.Get(jobID)
	if !ok {
		http.Error(w, "Unknown job", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderTemplate(w, r, "recording_archive_job_status.html", map[string]any{
		"Job":   snap,
		"JobID": jobID,
	})
}

// RecordingArchiveJobCancel cancels a running/queued archive job.
func (h *Handler) RecordingArchiveJobCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := normalizeArchiveJobID(r.URL.Query().Get("id"))
	if jobID == "" {
		if err := r.ParseForm(); err == nil {
			jobID = normalizeArchiveJobID(r.FormValue("id"))
		}
	}
	if jobID == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}
	if !h.archiveJobs.Cancel(jobID) {
		http.Error(w, "Job cannot be canceled", http.StatusBadRequest)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/recordings/archive/job?id="+url.QueryEscape(jobID), http.StatusFound)
}

func normalizeArchiveJobID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Some clients end up sending the id with surrounding quotes (id="...").
	// Accept it by unquoting once.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unq, err := strconv.Unquote(s); err == nil {
			return strings.TrimSpace(unq)
		}
		return strings.TrimSpace(strings.Trim(s, "\""))
	}
	return s
}

// Helper methods

func (h *Handler) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data any) {
	if data == nil {
		data = map[string]any{}
	}

	// Add common data
	if m, ok := data.(map[string]any); ok {
		m["User"] = r.Context().Value("user")
		m["Role"] = r.Context().Value("role")
		m["Year"] = time.Now().Year()
		m["Path"] = r.URL.Path
		m["ThemeDefault"] = h.uiThemeDefault
		m["ThemeMode"] = themeFromRequest(r, h.uiThemeDefault)
		m["BrandHref"] = h.landingPath()
		m["PageName"] = pageNameForPath(r.URL.Path)
	}

	// Set Content-Type header
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Get the specific template for this page
	tmpl, ok := h.templateMap[name]
	if !ok {
		h.logger.Error("template not found", slog.String("template", name))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Execute the template
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.logger.Error("template error", slog.Any("error", err), slog.String("template", name))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func normalizeTheme(theme string) string {
	switch theme {
	case "light", "dark", "system":
		return theme
	case "":
		return "system"
	default:
		return "system"
	}
}

func themeFromRequest(_ *http.Request, fallback string) string {
	// The UI theme is configured server-side (Configurations page) and should apply
	// consistently across all pages. Ignore any legacy per-browser theme cookie.
	return normalizeTheme(fallback)
}

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("handler error", slog.Any("error", err), slog.String("path", r.URL.Path))

	switch err {
	case domain.ErrNotFound:
		http.Error(w, "Not Found", http.StatusNotFound)
	case domain.ErrInvalidInput:
		http.Error(w, "Bad Request", http.StatusBadRequest)
	case domain.ErrUnauthorized:
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	case domain.ErrForbidden:
		http.Error(w, "Forbidden", http.StatusForbidden)
	default:
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
