package http

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RankingsV2Handler serves GET /api/rankings with sorting, pagination, and caching.
type RankingsV2Handler struct {
	useCase usecases.RankingsUseCase
}

// NewRankingsV2Handler constructs the handler.
func NewRankingsV2Handler(uc usecases.RankingsUseCase) *RankingsV2Handler {
	return &RankingsV2Handler{useCase: uc}
}

type rankingsV2Response struct {
	Timeframe  string                   `json:"timeframe"`
	Sort       string                   `json:"sort"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"pageSize"`
	TotalItems int                      `json:"totalItems"`
	TotalPages int                      `json:"totalPages"`
	Results    []rankedResultV2Response `json:"results"`
}

type rankedResultV2Response struct {
	Symbol     string             `json:"symbol"`
	TotalScore float64            `json:"totalScore"`
	Scores     map[string]float64 `json:"scores"`
	Volume     float64            `json:"volume"`
}

func (h *RankingsV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// --- Validate timeframe (required) ---
	tfStr := r.URL.Query().Get("timeframe")
	if tfStr == "" {
		writeRankingsError(w, "missing timeframe", http.StatusBadRequest)
		return
	}
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		writeRankingsError(w, "invalid timeframe", http.StatusBadRequest)
		return
	}

	// --- Parse sort (optional, default "total", unknown → "total") ---
	sortMode := usecases.ParseSortMode(r.URL.Query().Get("sort"))

	// --- Parse page (optional, default 1, clamp >=1) ---
	page := ParsePositiveIntOrDefault(r.URL.Query().Get("page"), 1)

	// --- Parse pageSize (optional, default 30, clamp 1–100) ---
	pageSize := ParsePositiveIntOrDefault(r.URL.Query().Get("pageSize"), 30)
	if pageSize > 100 {
		pageSize = 100
	}

	// --- Execute use case ---
	req := usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      sortMode,
	}
	results, err := h.useCase.Execute(r.Context(), req)
	if err != nil {
		writeRankingsError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// --- Pagination ---
	totalItems := len(results)
	totalPages := 0
	if totalItems > 0 {
		totalPages = int(math.Ceil(float64(totalItems) / float64(pageSize)))
	}

	start := (page - 1) * pageSize
	if start > totalItems {
		start = totalItems
	}
	end := start + pageSize
	if end > totalItems {
		end = totalItems
	}
	pageSlice := results[start:end]

	// --- Build response ---
	respResults := make([]rankedResultV2Response, 0, len(pageSlice))
	for _, r := range pageSlice {
		respResults = append(respResults, rankedResultV2Response{
			Symbol:     r.Symbol.String(),
			TotalScore: r.TotalScore,
			Scores:     r.Scores,
			Volume:     r.Volume,
		})
	}

	resp := rankingsV2Response{
		Timeframe:  tfStr,
		Sort:       string(sortMode),
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		Results:    respResults,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		writeRankingsError(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// parsePositiveIntOrDefault parses a string to a positive int, returning def on failure or <=0.
func ParsePositiveIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}

func writeRankingsError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
