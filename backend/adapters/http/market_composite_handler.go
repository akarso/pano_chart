package http

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	mkt "pano_chart/backend/domain/market"
)

// CompositeCalculator abstracts composite index computation
// so the handler works with both the direct service and cache decorator.
type CompositeCalculator interface {
	Calculate(ctx context.Context, timeframe string, limit int) (mkt.CompositeIndex, error)
}

// MarketCompositeHandler handles GET /api/market/composite requests.
type MarketCompositeHandler struct {
	service CompositeCalculator
}

// NewMarketCompositeHandler constructs the handler.
func NewMarketCompositeHandler(s CompositeCalculator) *MarketCompositeHandler {
	return &MarketCompositeHandler{service: s}
}

// compositeResponse is the JSON response DTO.
type compositeResponse struct {
	Timeframe   string          `json:"timeframe"`
	Points      []indexPointDTO `json:"points"`
	SymbolCount int             `json:"symbolCount"`
}

type indexPointDTO struct {
	T int64   `json:"t"`
	V float64 `json:"v"`
}

// ServeHTTP implements http.Handler.
func (h *MarketCompositeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "4h"
	}

	limit := 200
	if s := r.URL.Query().Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			if v > 500 {
				v = 500
			}
			limit = v
		}
	}

	index, err := h.service.Calculate(r.Context(), tf, limit)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	pts := make([]indexPointDTO, len(index.Points))
	for i, p := range index.Points {
		pts[i] = indexPointDTO{
			T: p.Timestamp,
			V: math.Round(p.Value*100) / 100, // 2 decimal places
		}
	}

	resp := compositeResponse{
		Timeframe:   index.Timeframe,
		Points:      pts,
		SymbolCount: index.SymbolCount,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
