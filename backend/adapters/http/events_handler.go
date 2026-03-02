package http

import (
	"encoding/json"
	"net/http"
	"time"

	"pano_chart/backend/application/usecases"
)

// EventsHandler handles GET /api/v1/events requests.
type EventsHandler struct {
	eventsUC usecases.EventsUseCase
}

// NewEventsHandler constructs the handler.
func NewEventsHandler(eventsUC usecases.EventsUseCase) *EventsHandler {
	return &EventsHandler{eventsUC: eventsUC}
}

// eventDTO is the response shape for a single event.
type eventDTO struct {
	ID        string `json:"id"`
	Country   string `json:"country"`
	Title     string `json:"title"`
	Impact    string `json:"impact"`
	Timestamp string `json:"timestamp"` // RFC3339 / ISO 8601
}

// eventsResponse is the top-level response.
type eventsResponse struct {
	Events []eventDTO `json:"events"`
}

const dateLayout = "2006-01-02"

// ServeHTTP implements http.Handler.
func (h *EventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	// Parse date_from (default: today - 1 day)
	now := time.Now().UTC()
	dateFrom := now.AddDate(0, 0, -1)
	if raw := q.Get("date_from"); raw != "" {
		parsed, err := time.Parse(dateLayout, raw)
		if err != nil {
			http.Error(w, `{"error":"invalid date_from, expected YYYY-MM-DD"}`, http.StatusBadRequest)
			return
		}
		dateFrom = parsed
	}

	// Parse date_to (default: today + 1 day)
	dateTo := now.AddDate(0, 0, 1)
	if raw := q.Get("date_to"); raw != "" {
		parsed, err := time.Parse(dateLayout, raw)
		if err != nil {
			http.Error(w, `{"error":"invalid date_to, expected YYYY-MM-DD"}`, http.StatusBadRequest)
			return
		}
		dateTo = parsed
	}

	if dateTo.Before(dateFrom) {
		http.Error(w, `{"error":"date_to must be after date_from"}`, http.StatusBadRequest)
		return
	}

	impact := q.Get("impact")
	country := q.Get("country")
	if country == "" {
		country = "United States"
	}

	req := usecases.GetEventsRequest{
		DateFrom: dateFrom,
		DateTo:   dateTo,
		Impact:   impact,
		Country:  country,
	}

	events, err := h.eventsUC.Execute(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtos := make([]eventDTO, len(events))
	for i, e := range events {
		dtos[i] = eventDTO{
			ID:        e.ID(),
			Country:   e.Country(),
			Title:     e.Title(),
			Impact:    string(e.Impact()),
			Timestamp: e.Timestamp().Format(time.RFC3339),
		}
	}

	resp := eventsResponse{Events: dtos}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"error":"encoding error"}`, http.StatusInternalServerError)
	}
}
