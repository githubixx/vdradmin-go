package http

import (
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/githubixx/vdradmin-go/internal/application/services"
	"github.com/githubixx/vdradmin-go/internal/domain"
)

// Handler handles HTTP requests
type Handler struct {
	logger           *slog.Logger
	templates        *template.Template
	templateMap      map[string]*template.Template
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
		epgService:       epgService,
		timerService:     timerService,
		recordingService: recordingService,
		autoTimerService: autoTimerService,
		uiThemeDefault:   "system",
	}
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

	var events []domain.EPGEvent
	if selected != "" {
		ev, err := h.epgService.GetEPG(r.Context(), selected, time.Now())
		if err != nil {
			h.logger.Error("EPG fetch error on channels", slog.Any("error", err), slog.String("channel", selected))
			data["HomeError"] = err.Error()
		} else {
			now := time.Now()
			for i := range ev {
				if ev[i].Stop.After(now) {
					events = append(events, ev[i])
				}
			}
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		return events[i].EventID < events[j].EventID
	})

	// Group by day, starting with current day.
	groups := make([]channelsDayGroup, 0)
	var currentDay time.Time
	for i := range events {
		day := time.Date(events[i].Start.Year(), events[i].Start.Month(), events[i].Start.Day(), 0, 0, 0, 0, events[i].Start.Location())
		if currentDay.IsZero() || !day.Equal(currentDay) {
			currentDay = day
			groups = append(groups, channelsDayGroup{Day: day})
		}
		groups[len(groups)-1].Events = append(groups[len(groups)-1].Events, events[i])
	}
	data["DayGroups"] = groups

	h.renderTemplate(w, r, "channels.html", data)
}

// Configurations renders a simple configuration page.
// For now it only allows switching between system/light/dark theme.
func (h *Handler) Configurations(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	h.renderTemplate(w, r, "configurations.html", data)
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
		"Query": query,
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
	if sortBy != "" {
		recordings = h.recordingService.SortRecordings(recordings, sortBy)
	}

	data := map[string]any{
		"Recordings": recordings,
		"Sort": sortBy,
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
