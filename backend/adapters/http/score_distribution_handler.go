package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	appscoring "pano_chart/backend/application/scoring"
)

// ScoreDistributionAPI is the debug query used by the distribution handler.
type ScoreDistributionAPI interface {
	Distribution(ctx context.Context, calculator, tf string) ([]float64, error)
	Calculators(ctx context.Context) ([]string, error)
}

// ScoreDistributionHandler serves GET /api/debug/score-distribution.
type ScoreDistributionHandler struct {
	api ScoreDistributionAPI
}

// NewScoreDistributionHandler constructs the handler.
func NewScoreDistributionHandler(api ScoreDistributionAPI) *ScoreDistributionHandler {
	return &ScoreDistributionHandler{api: api}
}

type scoreDistributionResponse struct {
	Calculator   string    `json:"calculator"`
	Timeframe    string    `json:"timeframe"`
	Distribution []float64 `json:"distribution"`
}

// ServeHTTP returns p5..p95 for one calculator and timeframe.
// This handler does not look at RemoteAddr. It is served on a loopback
// listener that is not the public API socket, so a same-host reverse proxy
// in front of the public port cannot reach it.
func (h *ScoreDistributionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeDistributionErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h == nil || h.api == nil {
		writeDistributionErr(w, http.StatusServiceUnavailable, "score distribution unavailable")
		return
	}
	q := r.URL.Query()
	calculator := strings.TrimSpace(q.Get("calculator"))
	tf := strings.ToLower(strings.TrimSpace(q.Get("timeframe")))
	if calculator == "" || tf == "" {
		writeDistributionErr(w, http.StatusBadRequest, "calculator and timeframe are required")
		return
	}
	dist, err := h.api.Distribution(r.Context(), calculator, tf)
	if errors.Is(err, appscoring.ErrNoSamples) {
		if h.writeIfUnknown(w, r, calculator) {
			return
		}
		writeDistributionErr(w, http.StatusNotFound, "no score samples")
		return
	}
	if err != nil {
		writeDistributionErr(w, http.StatusInternalServerError, "score distribution failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(scoreDistributionResponse{
		Calculator:   calculator,
		Timeframe:    tf,
		Distribution: dist,
	})
}

// writeIfUnknown reports 400 when calculator has no retained sample on any
// timeframe. A known calculator with no rows for the requested timeframe
// returns false so the caller can respond 404.
func (h *ScoreDistributionHandler) writeIfUnknown(w http.ResponseWriter, r *http.Request, calculator string) bool {
	names, err := h.api.Calculators(r.Context())
	if err != nil {
		writeDistributionErr(w, http.StatusInternalServerError, "score distribution failed")
		return true
	}
	for _, name := range names {
		if name == calculator {
			return false
		}
	}
	if names == nil {
		names = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(struct {
		Error       string   `json:"error"`
		Calculators []string `json:"calculators"`
	}{Error: "unknown calculator", Calculators: names})
	return true
}

func writeDistributionErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
