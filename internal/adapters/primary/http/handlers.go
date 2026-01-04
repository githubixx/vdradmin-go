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
		wanted := make(map[string]bool, len(h.cfg.VDR.WantedChannels))
		for _, id := range h.cfg.VDR.WantedChannels {
			wanted[id] = true
		}
		data["WantedChannelSet"] = wanted
	}
	if msg := strings.TrimSpace(r.URL.Query().Get("msg")); msg != "" {
		data["Message"] = msg
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("err")); errMsg != "" {
		data["Error"] = errMsg
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
		if s.UseChannel == "single" {
			if n := nameByID[s.ChannelID]; n != "" {
				label = n
			} else if s.ChannelID != "" {
				label = s.ChannelID
			}
		} else if s.UseChannel == "range" {
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

type epgSearchResultGroup struct {
	Search  epgSearchView
	Matches []domain.EPGEvent
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
				if !t.Active || strings.TrimSpace(t.ChannelID) == "" {
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
		lookupOccs := func() []timerOccurrence {
			if ev.ChannelNumber > 0 {
				if occs := occByChannelNumber[ev.ChannelNumber]; len(occs) > 0 {
					return occs
				}
			}
			if ev.ChannelID != "" {
				// Some VDR/SVDRP outputs don't provide the numeric channel in the EPG header.
				// When that happens, map the derived channel id -> number via the channels list.
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
				// Sometimes timers are indexed by numeric channel id string.
				if ev.ChannelNumber > 0 {
					if occs := occByChannelID[strconv.Itoa(ev.ChannelNumber)]; len(occs) > 0 {
						return occs
					}
				}
			}
			// Last resort: if we can map number->id, try that.
			if ev.ChannelNumber > 0 {
				if id := idByNumber[ev.ChannelNumber]; id != "" {
					if occs := occByChannelID[id]; len(occs) > 0 {
						return occs
					}
				}
			}
			return nil
		}

		if occs := lookupOccs(); len(occs) > 0 {
			evStart := ev.Start.In(loc)
			evStop := ev.Stop.In(loc)
			if evStop.Before(evStart) {
				evStop = evStart
			}
			for _, occ := range occs {
				// overlap if: occ.Start < evStop && evStart < occ.Stop
				if occ.Start.Before(evStop) && evStart.Before(occ.Stop) {
					active = true
					if t, ok := timersByID[occ.TimerID]; ok {
						if strings.TrimSpace(t.Title) != "" {
							label = t.Title
						} else {
							label = "active"
						}
					} else {
						label = "active"
					}
					break
				}
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

type epgSearchFormModel struct {
	Search   config.EPGSearch
	Channels []domain.Channel
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

func (h *Handler) EPGSearchCreate(w http.ResponseWriter, r *http.Request) {
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

	search := parseEPGSearchFromForm(r.PostForm)
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
	if h.cfg == nil {
		http.Error(w, "Configuration not available", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	search := parseEPGSearchFromForm(r.PostForm)
	if v := strings.TrimSpace(r.PostForm.Get("id")); v != "" {
		id, _ := strconv.Atoi(v)
		search.ID = id
	}
	if search.ID <= 0 {
		http.Error(w, "Invalid search id", http.StatusBadRequest)
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
		IsCritical  bool
		IsCollision bool
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

	// Mark overlapping timers (yellow) and critical timers (red) based on configured DVB cards.
	dvbCards := 1
	if h.cfg != nil && h.cfg.VDR.DVBCards > 0 {
		dvbCards = h.cfg.VDR.DVBCards
	}
	from := time.Now().In(time.Local)
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	to := from.Add(8 * 24 * time.Hour) // cover at least one full week for recurring timers

	collisionIDs, criticalIDs := timerOverlapStates(timers, dvbCards, from, to, func(t domain.Timer) string {
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

	dayStr := strings.TrimSpace(form.Get("day"))
	day, err := time.ParseInLocation("2006-01-02", dayStr, time.Local)
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	startStr := strings.TrimSpace(form.Get("start"))
	stopStr := strings.TrimSpace(form.Get("stop"))
	startClock, err1 := time.Parse("15:04", startStr)
	stopClock, err2 := time.Parse("15:04", stopStr)
	if err1 != nil || err2 != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
	stop := time.Date(day.Year(), day.Month(), day.Day(), stopClock.Hour(), stopClock.Minute(), 0, 0, time.Local)
	if stop.Before(start) {
		stop = stop.Add(24 * time.Hour)
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
		Priority:  priority,
		Lifetime:  lifetime,
		Title:     title,
		Aux:       aux,
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

	startStr := ""
	stopStr := ""
	if !t.Start.IsZero() {
		startStr = t.Start.In(time.Local).Format("15:04")
	}
	if !t.Stop.IsZero() {
		stopStr = t.Stop.In(time.Local).Format("15:04")
	}

	return timerFormModel{
		ID:        t.ID,
		Active:    t.Active,
		ChannelID: t.ChannelID,
		Day:       dayStr,
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

	dayStr := strings.TrimSpace(form.Get("day"))
	day, err := time.ParseInLocation("2006-01-02", dayStr, time.Local)
	if err != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	startStr := strings.TrimSpace(form.Get("start"))
	stopStr := strings.TrimSpace(form.Get("stop"))
	startClock, err1 := time.Parse("15:04", startStr)
	stopClock, err2 := time.Parse("15:04", stopStr)
	if err1 != nil || err2 != nil {
		return domain.Timer{}, domain.ErrInvalidInput
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
	stop := time.Date(day.Year(), day.Month(), day.Day(), stopClock.Hour(), stopClock.Minute(), 0, 0, time.Local)
	if stop.Before(start) {
		stop = stop.Add(24 * time.Hour)
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
		ID:        id,
		Active:    active,
		ChannelID: channelID,
		Day:       day,
		Start:     start,
		Stop:      stop,
		Priority:  priority,
		Lifetime:  lifetime,
		Title:     title,
		Aux:       aux,
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
