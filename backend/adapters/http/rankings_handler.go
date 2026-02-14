package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"strconv"
	"time"
)

type RankingsHandler struct {
	Ranker          usecases.RankSymbols
	CandleRepo      ports.CandleRepositoryPort
	Symbols         []domain.Symbol
	ExchangeInfoURL string
	TickerURL       string
	// For test stubs:
	TestSeries map[domain.Symbol]domain.CandleSeries // optional, for test wiring
}

type rankingsResponse struct {
	Timeframe string                 `json:"timeframe"`
	Count     int                    `json:"count"`
	Results   []rankedSymbolResponse `json:"results"`
}

type rankedSymbolResponse struct {
	Symbol     string             `json:"symbol"`
	TotalScore float64            `json:"totalScore"`
	Scores     map[string]float64 `json:"scores"`
}

func (h *RankingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tfStr := r.URL.Query().Get("timeframe")
	fmt.Printf("[rankings] Handler called, timeframe param: %s\n", tfStr)
	if tfStr == "" {
		fmt.Printf("[rankings] Missing timeframe param\n")
		http.Error(w, `{"error":"missing timeframe"}`, http.StatusBadRequest)
		return
	}
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		fmt.Printf("[rankings] Invalid timeframe: %s\n", tfStr)
		http.Error(w, `{"error":"invalid timeframe"}`, http.StatusBadRequest)
		return
	}
	limit := 30
	if lstr := r.URL.Query().Get("limit"); lstr != "" {
		l, err := strconv.Atoi(lstr)
		if err != nil || l <= 0 {
			fmt.Printf("[rankings] Invalid limit param: %s\n", lstr)
			http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
			return
		}
		limit = l
	}
	// Use test stub data if provided
	var series map[domain.Symbol]domain.CandleSeries
	if h.TestSeries != nil {
		series = h.TestSeries
		fmt.Printf("[rankings] Using test stub series, count=%d\n", len(series))
	} else {
		// Dynamically fetch universe symbols for each request
		universeProvider, ok := h.Ranker.(interface {
			Universe() usecases.SymbolUniverseProvider
		})
		var symbols []domain.Symbol
		if ok {
			syms, err := universeProvider.Universe().Symbols(r.Context(), h.ExchangeInfoURL, h.TickerURL)
			if err != nil {
				fmt.Printf("[rankings] Error fetching dynamic universe: %v\n", err)
				http.Error(w, `{"error":"universe fetch error"}`, http.StatusBadGateway)
				return
			}
			symbols = syms
		} else {
			fmt.Printf("[rankings] Ranker does not expose Universe provider, falling back to handler Symbols slice\n")
			symbols = h.Symbols
		}
		series = make(map[domain.Symbol]domain.CandleSeries)
		fmt.Printf("[rankings] Universe symbols: %d\n", len(symbols))
		for i, sym := range symbols {
			fmt.Printf("[rankings] [%d/%d] Fetching series for symbol=%s, timeframe=%s\n", i+1, len(symbols), sym.String(), tf.String())
			cs, err := h.CandleRepo.GetSeries(sym, tf, time.Time{}, time.Time{}) // TODO: set real time range
			if err != nil {
				fmt.Printf("[rankings] Error fetching series for %s: %v\n", sym.String(), err)
				continue
			}
			fmt.Printf("[rankings] Series fetched for %s: len=%d\n", sym.String(), cs.Len())
			if cs.Len() > 0 {
				candle, err := cs.At(0)
				if err == nil {
					fmt.Printf("[rankings] Sample candle for %s: %+v\n", sym.String(), candle)
				}
			} else {
				fmt.Printf("[rankings] No candles found for %s\n", sym.String())
			}
			series[sym] = cs
		}
		fmt.Printf("[rankings] Total series loaded: %d\n", len(series))
	}
	fmt.Printf("[rankings] Calling Ranker with series count: %d\n", len(series))
	ranked, err := h.Ranker.Rank(series)
	if err != nil {
		fmt.Printf("[rankings] Error ranking: %v\n", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	fmt.Printf("[rankings] Ranked count: %d\n", len(ranked))
	if len(ranked) > 0 {
		fmt.Printf("[rankings] Top ranked: symbol=%s, totalScore=%.4f, scores=%+v\n", ranked[0].Symbol.String(), ranked[0].TotalScore, ranked[0].Scores)
	} else {
		fmt.Printf("[rankings] No ranked symbols returned\n")
	}
	// Map to response DTOs
	resp := rankingsResponse{
		Timeframe: tfStr,
		Count:     min(limit, len(ranked)),
		Results:   make([]rankedSymbolResponse, 0, min(limit, len(ranked))),
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
		fmt.Printf("[rankings] Error encoding response: %v\n", err)
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
