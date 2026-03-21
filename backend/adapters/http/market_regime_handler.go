package http

import (
	"context"
	"encoding/json"
	"net/http"

	mkt "pano_chart/backend/domain/market"
)

// RegimeCalculator abstracts regime computation so the handler works with
// both the direct service and any cache decorator.
type RegimeCalculator interface {
	CalculateRegime(ctx context.Context, timeframe string) (mkt.RegimeSummary, error)
}

// MarketRegimeHandler handles GET /api/market/regime requests.
type MarketRegimeHandler struct {
	calculator RegimeCalculator
}

// NewMarketRegimeHandler constructs the handler.
func NewMarketRegimeHandler(c RegimeCalculator) *MarketRegimeHandler {
	return &MarketRegimeHandler{calculator: c}
}

// regimeResponse is the JSON response DTO.
type regimeResponse struct {
	Timeframe  string           `json:"timeframe"`
	Regime     string           `json:"regime"`
	Prevalence float64          `json:"prevalence"`
	Scores     regimeScoresDTO  `json:"scores"`
	Metrics    regimeMetricsDTO `json:"metrics"`
}

type regimeScoresDTO struct {
	Expansion   float64 `json:"expansion"`
	Compression float64 `json:"compression"`
	Trend       float64 `json:"trend"`
	Sideways    float64 `json:"sideways"`
}

type regimeMetricsDTO struct {
	TrendBreadth        float64 `json:"trendBreadth"`
	SidewaysBreadth     float64 `json:"sidewaysBreadth"`
	CompressionBreadth  float64 `json:"compressionBreadth"`
	BreakoutBreadth     float64 `json:"breakoutBreadth"`
	VolatilityExpansion float64 `json:"volatilityExpansion"`
	Dispersion          float64 `json:"dispersion"`
}

// ServeHTTP implements http.Handler.
func (h *MarketRegimeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "4h"
	}

	summary, err := h.calculator.CalculateRegime(r.Context(), tf)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	resp := regimeResponse{
		Timeframe:  summary.Timeframe,
		Regime:     string(summary.Regime),
		Prevalence: roundTo(summary.Prevalence, 4),
		Scores: regimeScoresDTO{
			Expansion:   roundTo(summary.Scores.Expansion, 4),
			Compression: roundTo(summary.Scores.Compression, 4),
			Trend:       roundTo(summary.Scores.Trend, 4),
			Sideways:    roundTo(summary.Scores.Sideways, 4),
		},
		Metrics: regimeMetricsDTO{
			TrendBreadth:        roundTo(summary.Metrics.TrendBreadth, 4),
			SidewaysBreadth:     roundTo(summary.Metrics.SidewaysBreadth, 4),
			CompressionBreadth:  roundTo(summary.Metrics.CompressionBreadth, 4),
			BreakoutBreadth:     roundTo(summary.Metrics.BreakoutBreadth, 4),
			VolatilityExpansion: roundTo(summary.Metrics.VolatilityExpansion, 4),
			Dispersion:          roundTo(summary.Metrics.Dispersion, 4),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
