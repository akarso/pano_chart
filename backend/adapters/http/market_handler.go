package http

import (
	"encoding/json"
	"math"
	"net/http"

	appmarket "pano_chart/backend/application/market"
)

// MarketHandler handles GET /api/market/state requests.
type MarketHandler struct {
	service *appmarket.MarketStateService
}

// NewMarketHandler constructs the handler.
func NewMarketHandler(s *appmarket.MarketStateService) *MarketHandler {
	return &MarketHandler{service: s}
}

// marketStateResponse is the response DTO for the market state endpoint.
type marketStateResponse struct {
	Timeframe      string           `json:"timeframe"`
	State          string           `json:"state"`
	Confidence     float64          `json:"confidence"`
	Breadth        marketBreadthDTO `json:"breadth"`
	SymbolCount    int              `json:"symbolCount"`
	Bias           string           `json:"bias"`
	EffectiveTrend float64          `json:"effectiveTrend"`
	BreakdownRate  float64          `json:"breakdownRate"`
	Label          string           `json:"label"`
}

type marketBreadthDTO struct {
	Sideways    float64 `json:"sideways"`
	Compression float64 `json:"compression"`
	Expansion   float64 `json:"expansion"`
	Trend       float64 `json:"trend"`
}

// ServeHTTP implements http.Handler.
func (h *MarketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "4h"
	}

	summary, err := h.service.Calculate(tf)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	resp := marketStateResponse{
		Timeframe:      summary.Timeframe,
		State:          string(summary.State),
		Confidence:     roundTo(summary.Confidence, 4),
		SymbolCount:    summary.SymbolCount,
		Bias:           summary.Bias,
		EffectiveTrend: roundTo(summary.EffectiveTrend, 4),
		BreakdownRate:  roundTo(summary.BreakdownRate, 4),
		Label:          summary.Label,
		Breadth: marketBreadthDTO{
			Sideways:    roundTo(summary.Breadth.Sideways, 4),
			Compression: roundTo(summary.Breadth.Compression, 4),
			Expansion:   roundTo(summary.Breadth.Expansion, 4),
			Trend:       roundTo(summary.Breadth.Trend, 4),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// roundTo rounds a float64 to n decimal places.
func roundTo(v float64, n int) float64 {
	pow := math.Pow(10, float64(n))
	return math.Round(v*pow) / pow
}
