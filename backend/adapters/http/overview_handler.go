package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// OverviewHandler handles GET /api/overview requests.
type OverviewHandler struct {
	getOverviewUC usecases.OverviewUseCase
}

// NewOverviewHandler constructs the handler.
func NewOverviewHandler(getOverviewUC usecases.OverviewUseCase) *OverviewHandler {
	return &OverviewHandler{getOverviewUC: getOverviewUC}
}

// overviewResponse is the response DTO for the overview endpoint.
type overviewResponse struct {
	Timeframe string              `json:"timeframe"`
	Count     int                 `json:"count"`
	Precision int                 `json:"precision"`
	Results   []overviewSymbolDTO `json:"results"`
}

// overviewSymbolDTO represents a single ranked symbol with sparkline.
type overviewSymbolDTO struct {
	Symbol     string    `json:"symbol"`
	TotalScore float64   `json:"totalScore"`
	Sparkline  []float64 `json:"sparkline"`
}

// ServeHTTP implements http.Handler.
func (h *OverviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse timeframe
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

	// Parse limit
	limit := 10 // default
	if lstr := r.URL.Query().Get("limit"); lstr != "" {
		l, err := strconv.Atoi(lstr)
		if err != nil || l <= 0 {
			http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
			return
		}
		limit = l
	}

	fmt.Printf("[overview] Handler called: timeframe=%s, limit=%d\n", tfStr, limit)

	// Execute use case
	req := usecases.GetOverviewRequest{
		Timeframe: tf,
		Limit:     limit,
	}

	results, err := h.getOverviewUC.Execute(r.Context(), req)
	if err != nil {
		fmt.Printf("[overview] Error executing use case: %v\n", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Map to DTOs
	dtoResults := make([]overviewSymbolDTO, len(results))
	for i, result := range results {
		dtoResults[i] = overviewSymbolDTO{
			Symbol:     result.Symbol.String(),
			TotalScore: result.TotalScore,
			Sparkline:  result.Sparkline,
		}
	}

	resp := overviewResponse{
		Timeframe: tfStr,
		Count:     len(dtoResults),
		Precision: len(results[0].Sparkline), // assumes all have same precision
		Results:   dtoResults,
	}

	if len(results) == 0 {
		resp.Precision = 0
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("[overview] Error encoding response: %v\n", err)
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
		return
	}

	fmt.Printf("[overview] Response sent: count=%d\n", resp.Count)
}
