package http

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	vol "pano_chart/backend/infrastructure/volatility"
)

// VolatilityHandler serves pre-computed intraday volatility profiles.
// It lazily loads the JSON from disk on the first request and caches
// the result in memory.
type VolatilityHandler struct {
	path string

	mu     sync.RWMutex
	cached *vol.FullResult
}

// NewVolatilityHandler constructs a handler backed by a JSON file.
func NewVolatilityHandler(jsonPath string) *VolatilityHandler {
	return &VolatilityHandler{path: jsonPath}
}

// ServeHTTP implements http.Handler.
func (h *VolatilityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}

	result, err := h.load()
	if err != nil {
		log.Printf("[volatility] failed to load data: %v", err)
		writeError(w, http.StatusServiceUnavailable, "DATA_UNAVAILABLE", "volatility data not available")
		return
	}

	// Extract 1m timeframe buckets (first entry).
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "1m"
	}

	for _, entry := range result.Intraday {
		if string(entry.Timeframe) == tf {
			writeJSON(w, http.StatusOK, entry)
			return
		}
	}

	writeError(w, http.StatusNotFound, "TIMEFRAME_NOT_FOUND", "timeframe not available")
}

// CurrentResult returns the currently-loaded volatility profile, loading it
// from disk on first use — the same cached data ServeHTTP reads. Exposed
// for VolatilitySeasonalityProvider (PR-082) to reuse this handler's
// existing load/cache mechanism instead of a second independent
// file-read/cache.
func (h *VolatilityHandler) CurrentResult() (*vol.FullResult, error) {
	return h.load()
}

func (h *VolatilityHandler) load() (*vol.FullResult, error) {
	h.mu.RLock()
	if h.cached != nil {
		defer h.mu.RUnlock()
		return h.cached, nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring write lock.
	if h.cached != nil {
		return h.cached, nil
	}

	result, err := h.fetch()
	if err != nil {
		return nil, err
	}
	h.cached = result
	return h.cached, nil
}

// fetch reads and parses the volatility profile from disk into a fresh
// value, without touching h.cached — the caller decides whether/when to
// commit it. Kept separate from load()/Reload() so neither has to inline
// (and risk silently dropping) the legacy-format migration below.
func (h *VolatilityHandler) fetch() (*vol.FullResult, error) {
	data, err := os.ReadFile(h.path)
	if err != nil {
		return nil, err
	}

	var result vol.FullResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// Fallback: if the file is the old flat format (just "buckets" at top
	// level, no "intraday" wrapper), wrap it as a single 1m timeframe.
	if len(result.Intraday) == 0 {
		var legacy vol.Result
		if err := json.Unmarshal(data, &legacy); err == nil && len(legacy.Buckets) > 0 {
			result.Intraday = []vol.TimeframeResult{
				{Timeframe: vol.TF1m, Buckets: legacy.Buckets},
			}
			log.Printf("[volatility] migrated legacy format (%d buckets) from %s", len(legacy.Buckets), h.path)
		}
	}

	log.Printf("[volatility] loaded %d timeframes from %s", len(result.Intraday), h.path)
	return &result, nil
}

// Reload forces a cache refresh from disk. Call this after vol_aggregate
// runs (or — since PR-082 — periodically via volatilityReloadLoop in
// cmd/api/main.go, now that this cache backs live setup confidence
// scoring, not just a display endpoint).
//
// Reads and parses into a local value BEFORE touching h.cached, and only
// swaps it in on success — CR follow-up: the previous version cleared
// h.cached up front, so a transient failure (a momentary disk hiccup, the
// file mid-write from a concurrent vol_aggregate run) permanently wiped
// previously-good data instead of preserving it, blacking out
// /api/volatility and silently degrading SeasonalityFit to neutral until
// the next successful reload — the same failure class already fixed for
// the futures OI cache in PR-081.
func (h *VolatilityHandler) Reload() error {
	result, err := h.fetch()
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.cached = result
	h.mu.Unlock()
	return nil
}
