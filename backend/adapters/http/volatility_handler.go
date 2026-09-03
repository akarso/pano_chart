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

	h.cached = &result
	log.Printf("[volatility] loaded %d timeframes from %s", len(result.Intraday), h.path)
	return h.cached, nil
}

// Reload forces a cache refresh from disk. Call this after vol_aggregate runs.
func (h *VolatilityHandler) Reload() error {
	h.mu.Lock()
	h.cached = nil
	h.mu.Unlock()
	_, err := h.load()
	return err
}
