package http

import (
   "encoding/json"
   "net/http"
   "strconv"
   "time"
   "pano_chart/backend/domain"
   "pano_chart/backend/application/usecases"
   "pano_chart/backend/application/ports"
)

type RankingsHandler struct {
	Ranker      usecases.RankSymbols
	CandleRepo  ports.CandleRepositoryPort
	Symbols     []domain.Symbol
	// For test stubs:
	TestSeries  map[domain.Symbol]domain.CandleSeries // optional, for test wiring
}

type rankingsResponse struct {
	Timeframe string                        `json:"timeframe"`
	Count     int                           `json:"count"`
	Results   []rankedSymbolResponse        `json:"results"`
}

type rankedSymbolResponse struct {
	Symbol     string             `json:"symbol"`
	TotalScore float64            `json:"totalScore"`
	Scores     map[string]float64 `json:"scores"`
}

func (h *RankingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
   tfStr := r.URL.Query().Get("timeframe")
   if tfStr == "" {
	   http.Error(w, `{"error":"missing timeframe"}`, http.StatusBadRequest)
	   return
   }
   tf, err := domain.NewTimeframe(tfStr)
   if err != nil {
	   http.Error(w, `{"error":"invalid timeframe"}`, http.StatusBadRequest)
	   return
   }
   limit := 30
   if lstr := r.URL.Query().Get("limit"); lstr != "" {
	   l, err := strconv.Atoi(lstr)
	   if err != nil || l <= 0 {
		   http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
		   return
	   }
	   limit = l
   }
   // Use test stub data if provided
   var series map[domain.Symbol]domain.CandleSeries
   if h.TestSeries != nil {
	   series = h.TestSeries
   } else {
	   // Load CandleSeries for all symbols (for now, use full available range)
	   series = make(map[domain.Symbol]domain.CandleSeries)
	   for _, sym := range h.Symbols {
		cs, err := h.CandleRepo.GetSeries(sym, tf, /*from*/time.Time{}, /*to*/time.Time{}) // TODO: set real time range
		   if err != nil {
			   http.Error(w, `{"error":"upstream data error"}`, http.StatusBadGateway)
			   return
		   }
		   series[sym] = cs
	   }
   }
   ranked, err := h.Ranker.Rank(series)
   if err != nil {
	   http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
	   return
   }
   // Map to response DTOs
   resp := rankingsResponse{
	   Timeframe: tfStr,
	   Count: min(limit, len(ranked)),
	   Results: make([]rankedSymbolResponse, 0, min(limit, len(ranked))),
   }
   for i := 0; i < resp.Count; i++ {
	   r := ranked[i]
	   resp.Results = append(resp.Results, rankedSymbolResponse{
		   Symbol:     r.Symbol.String(),
		   TotalScore: r.TotalScore,
		   Scores:     r.Scores,
	   })
   }
   w.Header().Set("Content-Type", "application/json")
   if err := json.NewEncoder(w).Encode(resp); err != nil {
	   http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	   return
   }
}

func min(a, b int) int {
   if a < b {
	   return a
   }
   return b
}
