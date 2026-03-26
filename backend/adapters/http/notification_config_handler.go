package http

import (
	"encoding/json"
	"net/http"

	appnotify "pano_chart/backend/application/notifications"
)

// NotificationConfigHandler handles GET/PUT /api/notification/config.
type NotificationConfigHandler struct {
	store appnotify.NotificationConfigStore
}

// NewNotificationConfigHandler constructs the handler.
func NewNotificationConfigHandler(store appnotify.NotificationConfigStore) *NotificationConfigHandler {
	return &NotificationConfigHandler{store: store}
}

type notificationConfigDTO struct {
	UserID                string  `json:"user_id"`
	Social                bool    `json:"social"`
	Macro                 bool    `json:"macro"`
	News                  bool    `json:"news"`
	Uptrend               bool    `json:"uptrend"`
	Downtrend             bool    `json:"downtrend"`
	Sideways              bool    `json:"sideways"`
	SetupOfDay            bool    `json:"setup_of_day"`
	UptrendMinDominance   float64 `json:"uptrend_min_dominance"`
	DowntrendMinDominance float64 `json:"downtrend_min_dominance"`
	SidewaysMinDominance  float64 `json:"sideways_min_dominance"`
	SetupMinScore         float64 `json:"setup_min_score"`
	UptrendTimeframe      string  `json:"uptrend_timeframe,omitempty"`
	DowntrendTimeframe    string  `json:"downtrend_timeframe,omitempty"`
	SidewaysTimeframe     string  `json:"sideways_timeframe,omitempty"`
	SetupTimeframe        string  `json:"setup_timeframe,omitempty"`
}

func toDTO(cfg appnotify.NotificationConfig) notificationConfigDTO {
	return notificationConfigDTO{
		UserID:                cfg.UserID,
		Social:                cfg.Social,
		Macro:                 cfg.Macro,
		News:                  cfg.News,
		Uptrend:               cfg.Uptrend,
		Downtrend:             cfg.Downtrend,
		Sideways:              cfg.Sideways,
		SetupOfDay:            cfg.SetupOfDay,
		UptrendMinDominance:   cfg.UptrendMinDominance,
		DowntrendMinDominance: cfg.DowntrendMinDominance,
		SidewaysMinDominance:  cfg.SidewaysMinDominance,
		SetupMinScore:         cfg.SetupMinScore,
		UptrendTimeframe:      cfg.UptrendTimeframe,
		DowntrendTimeframe:    cfg.DowntrendTimeframe,
		SidewaysTimeframe:     cfg.SidewaysTimeframe,
		SetupTimeframe:        cfg.SetupTimeframe,
	}
}

func fromDTO(dto notificationConfigDTO) appnotify.NotificationConfig {
	return appnotify.NotificationConfig{
		UserID:                dto.UserID,
		Social:                dto.Social,
		Macro:                 dto.Macro,
		News:                  dto.News,
		Uptrend:               dto.Uptrend,
		Downtrend:             dto.Downtrend,
		Sideways:              dto.Sideways,
		SetupOfDay:            dto.SetupOfDay,
		UptrendMinDominance:   dto.UptrendMinDominance,
		DowntrendMinDominance: dto.DowntrendMinDominance,
		SidewaysMinDominance:  dto.SidewaysMinDominance,
		SetupMinScore:         dto.SetupMinScore,
		UptrendTimeframe:      dto.UptrendTimeframe,
		DowntrendTimeframe:    dto.DowntrendTimeframe,
		SidewaysTimeframe:     dto.SidewaysTimeframe,
		SetupTimeframe:        dto.SetupTimeframe,
	}
}

// ServeHTTP implements http.Handler.
func (h *NotificationConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handlePut(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *NotificationConfigHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
		return
	}

	cfg, err := h.store.Get(userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toDTO(cfg))
}

func (h *NotificationConfigHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	var dto notificationConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if dto.UserID == "" {
		http.Error(w, `{"error":"user_id is required"}`, http.StatusBadRequest)
		return
	}

	if err := h.store.Save(fromDTO(dto)); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}
