package http

import (
	"context"
	"encoding/json"
	"net/http"

	mkt "pano_chart/backend/domain/market"
)

// TransitionCalculator abstracts transition-probability computation.
type TransitionCalculator interface {
	Calculate(ctx context.Context, timeframe string) (mkt.MarketTransition, error)
}

// MarketTransitionHandler handles GET /api/market/transition requests.
type MarketTransitionHandler struct {
	calculator TransitionCalculator
}

// NewMarketTransitionHandler constructs the handler.
func NewMarketTransitionHandler(c TransitionCalculator) *MarketTransitionHandler {
	return &MarketTransitionHandler{calculator: c}
}

// transitionResponse is the JSON response DTO.
type transitionResponse struct {
	Timeframe     string           `json:"timeframe"`
	CurrentRegime string           `json:"currentRegime"`
	Probabilities probabilitiesDTO `json:"probabilities"`
	Horizon       string           `json:"horizon"`
}

type probabilitiesDTO struct {
	Trend       float64 `json:"trend"`
	Sideways    float64 `json:"sideways"`
	Compression float64 `json:"compression"`
	Expansion   float64 `json:"expansion"`
}

// ServeHTTP implements http.Handler.
func (h *MarketTransitionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "4h"
	}

	result, err := h.calculator.Calculate(r.Context(), tf)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	resp := transitionResponse{
		Timeframe:     result.Timeframe,
		CurrentRegime: string(result.CurrentRegime),
		Probabilities: probabilitiesDTO{
			Trend:       roundTo(result.Probabilities.Trend, 4),
			Sideways:    roundTo(result.Probabilities.Sideways, 4),
			Compression: roundTo(result.Probabilities.Compression, 4),
			Expansion:   roundTo(result.Probabilities.Expansion, 4),
		},
		Horizon: result.Horizon,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
