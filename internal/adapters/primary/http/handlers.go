package http

import (
	"html/template"
	"log/slog"
	"net/http"
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
	}
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

// EPGList shows EPG listing
func (h *Handler) EPGList(w http.ResponseWriter, r *http.Request) {
	channelID := r.URL.Query().Get("channel")

	events, err := h.epgService.GetEPG(r.Context(), channelID, time.Now())
	if err != nil {
		h.logger.Error("EPG fetch error", slog.Any("error", err), slog.String("channel", channelID))
		h.handleError(w, r, err)
		return
	}

	h.logger.Info("EPG fetched", slog.Int("event_count", len(events)), slog.String("channel", channelID))

	data := map[string]any{
		"Events": events,
		"Channel": channelID,
	}

	h.renderTemplate(w, r, "epg.html", data)
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
	if query == "" {
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

	h.renderTemplate(w, r, "search.html", data)
}

// TimerList shows all timers
func (h *Handler) TimerList(w http.ResponseWriter, r *http.Request) {
	timers, err := h.timerService.GetAllTimers(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	data := map[string]any{
		"Timers": timers,
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
	if eventID > 0 {
		// Create from EPG
		events, err := h.epgService.GetEPG(r.Context(), "", time.Time{})
		if err != nil {
			h.handleError(w, r, err)
			return
		}

		for _, event := range events {
			if event.EventID == eventID {
				err := h.timerService.CreateTimerFromEPG(r.Context(), event, 50, 99, 2, 10)
				if err != nil {
					h.handleError(w, r, err)
					return
				}

				w.Header().Set("HX-Redirect", "/timers")
				w.WriteHeader(http.StatusOK)
				return
			}
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
