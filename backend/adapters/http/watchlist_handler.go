package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

type watchlistRequest struct {
	Symbols []string `json:"symbols"`
}

type watchlistResponse struct {
	Symbols []string `json:"symbols"`
}

// WatchlistHandler handles GET/PUT/DELETE /api/watchlist. Must be
// registered via NewWatchlistRoute (not authMW/hand-wired RequireAuth) —
// see that constructor's doc for why this endpoint hard-enforces auth
// unconditionally.
type WatchlistHandler struct {
	store ports.WatchlistStore
}

// NewWatchlistHandler constructs the handler.
func NewWatchlistHandler(store ports.WatchlistStore) *WatchlistHandler {
	return &WatchlistHandler{store: store}
}

// ServeHTTP implements http.Handler.
func (h *WatchlistHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPut:
		h.handlePut(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *WatchlistHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	symbols, err := h.store.Get(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.writeSymbols(w, symbols)
}

func (h *WatchlistHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	var req watchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	symbols, err := normalizeSymbols(req.Symbols)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	if err := h.store.Replace(r.Context(), userID, symbols); err != nil {
		if errors.Is(err, ports.ErrWatchlistTooLarge) {
			http.Error(w, fmt.Sprintf(`{"error":"too many symbols, max %d"}`, ports.WatchlistMaxSymbols), http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	symbols, err = h.store.Get(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.writeSymbols(w, symbols)
}

func (h *WatchlistHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	var req watchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	symbols, err := normalizeSymbols(req.Symbols)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	if err := h.store.Remove(r.Context(), userID, symbols); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	symbols, err = h.store.Get(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.writeSymbols(w, symbols)
}

// normalizeSymbols validates and canonicalizes each raw symbol string via
// domain.NewSymbol (non-empty, valid chars, uppercased) and de-duplicates
// the result (preserving first-seen order) — so "btcusdt" and "BTCUSDT"
// collapse to one watchlist entry instead of two, and a WatchlistMaxSymbols
// cap check downstream counts distinct symbols rather than raw request
// entries. Returns an error naming the first invalid entry.
func normalizeSymbols(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if len(s) > ports.WatchlistMaxSymbolLength {
			return nil, fmt.Errorf("symbol too long (max %d chars): %q", ports.WatchlistMaxSymbolLength, s)
		}
		sym, err := domain.NewSymbol(s)
		if err != nil {
			return nil, fmt.Errorf("invalid symbol %q: %w", s, err)
		}
		norm := sym.String()
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out, nil
}

func (h *WatchlistHandler) writeSymbols(w http.ResponseWriter, symbols []string) {
	if symbols == nil {
		symbols = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(watchlistResponse{Symbols: symbols})
}

// WatchlistRateLimitPerHour / Burst bound how often one authenticated user
// can hit /api/watchlist (PR-101 CR) — matches the PR-075 precedent of
// rate-limiting every mutating endpoint (see payment_handler.go's
// VerifyPurchaseRateLimit*), which this endpoint had originally shipped
// without. Burst covers a realistic UI burst — resyncing a watchlist of
// several newly-starred symbols in one interaction — without leaving the
// door open to hammering SQLite writes.
const (
	WatchlistRateLimitPerHour = 120
	WatchlistRateLimitBurst   = 15
)

// NewWatchlistRoute wires the production handler chain for
// /api/watchlist: auth is HARD-enforced (enforce=true) here, independent
// of the general AUTH_ENFORCE env var that most other endpoints share —
// this is a brand-new endpoint (ROADMAP PR-101) with no pre-auth client
// to migrate, so there is no reason to run it in log-only mode the way
// notification_config_handler.go and friends must for backward
// compatibility. cmd/api/main.go must wire this route through this
// constructor, not by hand-assembling middleware.RequireAuth(..., false)
// or the shared authMW.
//
// Rate limiting is the innermost layer, applied to the handler before
// RequireAuth wraps it — PerUserRateLimit reads the authenticated user ID
// from context, which only exists once RequireAuth has already run.
func NewWatchlistRoute(store ports.WatchlistStore, credStore ports.CredentialStore) http.Handler {
	limited := middleware.PerUserRateLimit(WatchlistRateLimitPerHour, WatchlistRateLimitBurst)(NewWatchlistHandler(store))
	return middleware.RequireAuth(credStore, true)(limited)
}
