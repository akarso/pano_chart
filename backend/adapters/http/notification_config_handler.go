package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"pano_chart/backend/adapters/http/middleware"
	appeval "pano_chart/backend/application/evaluation"
	appnotify "pano_chart/backend/application/notifications"
)

// NotificationConfigHandler handles GET/PUT /api/notification/config.
// Must be registered behind middleware.RequireAuth.
type NotificationConfigHandler struct {
	store appnotify.NotificationConfigStore
}

// NewNotificationConfigHandler constructs the handler.
func NewNotificationConfigHandler(store appnotify.NotificationConfigStore) *NotificationConfigHandler {
	return &NotificationConfigHandler{store: store}
}

type notificationConfigDTO struct {
	UserID        string `json:"user_id"`
	Social        bool   `json:"social"`
	Macro         *bool  `json:"macro,omitempty"`
	MacroHigh     bool   `json:"macro_high"`
	MacroModerate bool   `json:"macro_moderate"`
	News          bool   `json:"news"`
	Uptrend       bool   `json:"uptrend"`
	Downtrend     bool   `json:"downtrend"`
	Sideways      bool   `json:"sideways"`
	SetupOfDay    bool   `json:"setup_of_day"`
	// WatchlistTransitions is a pointer, unlike every other bool field
	// here (PR-101 CR) — an app built before this field existed sends a
	// PUT body without a "watchlist_transitions" key at all, and a plain
	// bool can't tell that apart from an explicit `false`. Decoding such a
	// body would otherwise silently overwrite the migrated/default `true`
	// with `false` on every unrelated setting change, permanently, since
	// that write also stamps the current config_version — the same
	// migration guard that protects a genuinely untouched row no longer
	// applies once the row has been (re)saved. nil means "this client
	// didn't send an opinion" — handlePut preserves the existing stored
	// value in that case instead of defaulting to false.
	WatchlistTransitions  *bool   `json:"watchlist_transitions,omitempty"`
	UptrendMinDominance   float64 `json:"uptrend_min_dominance"`
	DowntrendMinDominance float64 `json:"downtrend_min_dominance"`
	SidewaysMinDominance  float64 `json:"sideways_min_dominance"`
	SetupMinScore         float64 `json:"setup_min_score"`
	UptrendTimeframe      string  `json:"uptrend_timeframe,omitempty"`
	DowntrendTimeframe    string  `json:"downtrend_timeframe,omitempty"`
	SidewaysTimeframe     string  `json:"sideways_timeframe,omitempty"`
	SetupTimeframe        string  `json:"setup_timeframe,omitempty"`
	WatchlistTimeframe    string  `json:"watchlist_timeframe,omitempty"`
}

func toDTO(cfg appnotify.NotificationConfig) notificationConfigDTO {
	watchlistTransitions := cfg.WatchlistTransitions
	return notificationConfigDTO{
		UserID:                cfg.UserID,
		Social:                cfg.Social,
		MacroHigh:             cfg.MacroHigh,
		MacroModerate:         cfg.MacroModerate,
		News:                  cfg.News,
		Uptrend:               cfg.Uptrend,
		Downtrend:             cfg.Downtrend,
		Sideways:              cfg.Sideways,
		SetupOfDay:            cfg.SetupOfDay,
		WatchlistTransitions:  &watchlistTransitions,
		UptrendMinDominance:   cfg.UptrendMinDominance,
		DowntrendMinDominance: cfg.DowntrendMinDominance,
		SidewaysMinDominance:  cfg.SidewaysMinDominance,
		SetupMinScore:         cfg.SetupMinScore,
		UptrendTimeframe:      cfg.UptrendTimeframe,
		DowntrendTimeframe:    cfg.DowntrendTimeframe,
		SidewaysTimeframe:     cfg.SidewaysTimeframe,
		SetupTimeframe:        cfg.SetupTimeframe,
		WatchlistTimeframe:    cfg.WatchlistTimeframe,
	}
}

// fromDTO builds the config to save from dto, falling back to existing
// (the caller's currently-stored config, or DefaultNotificationConfig for a
// brand-new user) for any field a client can leave unstated. Today that's
// only WatchlistTransitions (PR-101 CR) — every other field here predates
// the pointer-DTO pattern and a client always sends its full opinion on
// them.
func fromDTO(dto notificationConfigDTO, existing appnotify.NotificationConfig) appnotify.NotificationConfig {
	// Migrate legacy "macro" bool from old clients.
	macroHigh := dto.MacroHigh
	macroMod := dto.MacroModerate
	if dto.Macro != nil {
		macroHigh = *dto.Macro
		macroMod = *dto.Macro
	}
	watchlistTransitions := existing.WatchlistTransitions
	if dto.WatchlistTransitions != nil {
		watchlistTransitions = *dto.WatchlistTransitions
	}
	return appnotify.NotificationConfig{
		UserID:                dto.UserID,
		Social:                dto.Social,
		MacroHigh:             macroHigh,
		MacroModerate:         macroMod,
		News:                  dto.News,
		Uptrend:               dto.Uptrend,
		Downtrend:             dto.Downtrend,
		Sideways:              dto.Sideways,
		SetupOfDay:            dto.SetupOfDay,
		WatchlistTransitions:  watchlistTransitions,
		UptrendMinDominance:   dto.UptrendMinDominance,
		DowntrendMinDominance: dto.DowntrendMinDominance,
		SidewaysMinDominance:  dto.SidewaysMinDominance,
		SetupMinScore:         dto.SetupMinScore,
		UptrendTimeframe:      dto.UptrendTimeframe,
		DowntrendTimeframe:    dto.DowntrendTimeframe,
		SidewaysTimeframe:     dto.SidewaysTimeframe,
		SetupTimeframe:        dto.SetupTimeframe,
		WatchlistTimeframe:    dto.WatchlistTimeframe,
	}
}

// isValidWatchlistTimeframe reports whether tf is one of the timeframes
// application/mtf.Service actually produces frames for (PR-101 CR): the
// regime stack is hard-coded to appeval.DefaultTimeframes, so accepting
// anything else (e.g. "1m"/"5m", valid for every *other* timeframe field
// on this config) would silently and permanently never find a matching
// frame — checkWatchlistSymbol would return early on every single scan,
// forever, with no error surfaced anywhere.
func isValidWatchlistTimeframe(tf string) bool {
	for _, valid := range appeval.DefaultTimeframes {
		if tf == valid {
			return true
		}
	}
	return false
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
	userID, ok := middleware.UserIDOrLegacyFallback(r.Context(), r.URL.Query().Get("user_id"))
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
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

	userID, ok := middleware.UserIDOrLegacyFallback(r.Context(), dto.UserID)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	dto.UserID = userID // authenticated case: overrides whatever the client sent

	if dto.WatchlistTimeframe != "" && !isValidWatchlistTimeframe(dto.WatchlistTimeframe) {
		http.Error(w, fmt.Sprintf(`{"error":"invalid watchlist_timeframe, must be one of %v"}`, appeval.DefaultTimeframes), http.StatusBadRequest)
		return
	}

	existing, err := h.store.Get(userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if err := h.store.Save(fromDTO(dto, existing)); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}
