package http

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
	epgService       *services.EPGService
	timerService     *services.TimerService
	recordingService *services.RecordingService
	autoTimerService *services.AutoTimerService
	uiThemeDefault   string
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
	events, err := h.epgService.GetCurrentPrograms(r.Context())
	data := map[string]any{}
	if err != nil {
		// Keep the UI reachable even if VDR/SVDRP is unavailable.
		h.logger.Error("EPG fetch error on home", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["Events"] = []domain.EPGEvent{}
	} else {
		data["Events"] = events
	}

	h.renderTemplate(w, r, "index.html", data)
}

type channelsDayGroup struct {
	Day    time.Time
	Events []domain.EPGEvent
}

type channelsDayOption struct {
	Value string
	Label string
}

type playingChannelGroup struct {
	Channel domain.Channel
	Events  []domain.EPGEvent
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

	loc := time.Now().Location()
	dayStart, err := parseDayParam(r, loc)
	if err != nil {
		h.logger.Error("invalid day parameter", slog.Any("error", err))
		dayStart = time.Now().In(loc)
		dayStart = time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, loc)
	}
	dayEnd := dayStart.Add(24 * time.Hour)
	data["SelectedDay"] = dayStart.Format("2006-01-02")

	var events []domain.EPGEvent
	daysByValue := map[string]time.Time{}
	// Always include selected day in the dropdown.
	daysByValue[dayStart.Format("2006-01-02")] = dayStart
	if selected != "" {
		ev, err := h.epgService.GetEPG(r.Context(), selected, time.Now())
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
		data["DayGroups"] = []channelsDayGroup{{Day: dayStart, Events: events}}
	} else {
		data["DayGroups"] = []channelsDayGroup{}
	}

	h.renderTemplate(w, r, "channels.html", data)
}

// Configurations renders a simple configuration page.
// For now it only allows switching between system/light/dark theme.
func (h *Handler) Configurations(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if h.cfg != nil {
		data["Config"] = h.cfg
	}
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		data["Message"] = msg
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("err")); errMsg != "" {
		data["Error"] = errMsg
	}
	h.renderTemplate(w, r, "configurations.html", data)
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
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  err.Error(),
		})
		return
	}

	if err := h.applyRuntimeConfig(updated); err != nil {
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  err.Error(),
		})
		return
	}

	setThemeCookie(w, updated.UI.Theme)

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
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  err.Error(),
		})
		return
	}

	restartRequired := serverRestartRequired(h.cfg.Server, updated.Server)

	// Persist
	if strings.TrimSpace(h.configPath) == "" {
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  "No config path configured; cannot save.",
		})
		return
	}
	if err := updated.Save(h.configPath); err != nil {
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  err.Error(),
		})
		return
	}

	if err := h.applyRuntimeConfig(updated); err != nil {
		h.renderTemplate(w, r, "configurations.html", map[string]any{
			"Config": h.cfg,
			"Error":  err.Error(),
		})
		return
	}

	setThemeCookie(w, updated.UI.Theme)

	msg := "Saved and applied configuration."
	if restartRequired {
		msg = "Saved and applied configuration (requires daemon restart)."
	}
	http.Redirect(w, r, "/configurations?msg="+url.QueryEscape(msg), http.StatusSeeOther)
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

	// VDR connection
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

func setThemeCookie(w http.ResponseWriter, theme string) {
	theme = normalizeTheme(strings.TrimSpace(theme))
	// Persist explicit mode (including system) for a long time.
	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    url.QueryEscape(theme),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		SameSite: http.SameSiteLaxMode,
	})
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

	allEvents, err := h.epgService.GetEPG(r.Context(), "", dayStart)
	if err != nil {
		h.logger.Error("EPG fetch error on playing today", slog.Any("error", err))
		data["HomeError"] = err.Error()
		data["ChannelGroups"] = []playingChannelGroup{}
		h.renderTemplate(w, r, "playing.html", data)
		return
	}

	eventsByChannel := make(map[string][]domain.EPGEvent)
	for i := range allEvents {
		// Keep events that overlap with the selected day.
		if allEvents[i].Start.Before(dayEnd) && allEvents[i].Stop.After(dayStart) {
			eventsByChannel[allEvents[i].ChannelID] = append(eventsByChannel[allEvents[i].ChannelID], allEvents[i])
		}
	}

	groups := make([]playingChannelGroup, 0, len(channels))
	for _, ch := range channels {
		ev := eventsByChannel[ch.ID]
		if len(ev) == 0 {
			continue
		}
		sort.SliceStable(ev, func(i, j int) bool {
			if !ev[i].Start.Equal(ev[j].Start) {
				return ev[i].Start.Before(ev[j].Start)
			}
			return ev[i].EventID < ev[j].EventID
		})
		groups = append(groups, playingChannelGroup{Channel: ch, Events: ev})
	}

	data["ChannelGroups"] = groups
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

	data := map[string]any{
		"Query":  query,
		"Events": events,
	}

	if isHTMX {
		h.renderTemplate(w, r, "search_results.html", data)
		return
	}

	h.renderTemplate(w, r, "search.html", data)
}

// TimerList shows all timers
func (h *Handler) TimerList(w http.ResponseWriter, r *http.Request) {
	timers, err := h.timerService.GetAllTimers(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	channels, chErr := h.epgService.GetChannels(r.Context())
	nameByID := map[string]string{}
	if chErr == nil {
		for _, ch := range channels {
			if ch.ID != "" {
				nameByID[ch.ID] = ch.Name
			}
			if ch.Number != 0 {
				nameByID[strconv.Itoa(ch.Number)] = ch.Name
			}
		}
	}

	type timerView struct {
		domain.Timer
		ChannelName string
		IsRecording bool
	}

	views := make([]timerView, 0, len(timers))
	for _, t := range timers {
		name := nameByID[t.ChannelID]
		if name == "" {
			name = t.ChannelID
		}
		isRec := false
		if !t.Start.IsZero() && !t.Stop.IsZero() {
			now := time.Now()
			isRec = (t.Start.Before(now) || t.Start.Equal(now)) && t.Stop.After(now)
		}
		views = append(views, timerView{Timer: t, ChannelName: name, IsRecording: isRec})
	}

	now := time.Now()
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

	data := map[string]any{
		"Timers": views,
	}

	h.renderTemplate(w, r, "timers.html", data)
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
			err := h.timerService.CreateTimerFromEPG(r.Context(), event, 50, 99, 2, 10)
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

	w.WriteHeader(http.StatusOK)
}

// RecordingList shows all recordings
func (h *Handler) RecordingList(w http.ResponseWriter, r *http.Request) {
	recordings, err := h.recordingService.GetAllRecordings(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "date"
	}
	recordings = h.recordingService.SortRecordings(recordings, sortBy)

	data := map[string]any{
		"Recordings": recordings,
		"Sort":       sortBy,
	}

	h.renderTemplate(w, r, "recordings.html", data)
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

func themeFromRequest(r *http.Request, fallback string) string {
	fallback = normalizeTheme(fallback)
	c, err := r.Cookie("theme")
	if err != nil {
		return fallback
	}
	return normalizeTheme(c.Value)
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
