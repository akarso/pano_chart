package http

import (
	"encoding/json"
	"net/http"

	mkt "pano_chart/backend/domain/market"
)

// HistoryProvider abstracts regime history retrieval.
type HistoryProvider interface {
	GetHistory(timeframe string, limit int) (mkt.RegimeHistory, error)
}

// MarketRegimeHistoryHandler handles GET /api/market/regime/history requests.
type MarketRegimeHistoryHandler struct {
	provider HistoryProvider
}

// NewMarketRegimeHistoryHandler constructs the handler.
func NewMarketRegimeHistoryHandler(p HistoryProvider) *MarketRegimeHistoryHandler {
	return &MarketRegimeHistoryHandler{provider: p}
}

// regimeHistoryResponse is the JSON response DTO.
type regimeHistoryResponse struct {
	Timeframe  string            `json:"timeframe"`
	History    []regimePeriodDTO `json:"history"`
	CurrentAge int               `json:"currentAge"`
}

type regimePeriodDTO struct {
	Regime          string `json:"regime"`
	Start           int64  `json:"start"`
	End             *int64 `json:"end"`
	DurationCandles int    `json:"durationCandles"`
}

// ServeHTTP implements http.Handler.
func (h *MarketRegimeHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "4h"
	}

	history, err := h.provider.GetHistory(tf, 50)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	periods := make([]regimePeriodDTO, len(history.Periods))
	for i, p := range history.Periods {
		periods[i] = regimePeriodDTO{
			Regime:          string(p.Regime),
			Start:           p.StartTimestamp,
			End:             p.EndTimestamp,
			DurationCandles: p.DurationCandles,
		}
	}

	resp := regimeHistoryResponse{
		Timeframe:  history.Timeframe,
		History:    periods,
		CurrentAge: history.CurrentAge,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
