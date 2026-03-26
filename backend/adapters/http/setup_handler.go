package http

import (
	"context"
	"net/http"
	"strings"

	"pano_chart/backend/domain/setup"
)

const setupPrefix = "/api/token/"
const setupSuffix = "/setup"

// SetupUseCase defines the boundary the handler depends on.
type SetupUseCase interface {
	Evaluate(ctx context.Context, symbol, timeframe string) (setup.SetupScores, error)
}

// SetupHandler handles GET /api/token/{symbol}/setup requests.
type SetupHandler struct {
	uc SetupUseCase
}

// NewSetupHandler constructs the handler.
func NewSetupHandler(uc SetupUseCase) *SetupHandler {
	return &SetupHandler{uc: uc}
}

// ServeHTTP implements http.Handler.
func (h *SetupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}

	symbolStr, ok := extractSetupSymbol(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /api/token/{symbol}/setup")
		return
	}

	if _, err := ParseSymbol(symbolStr); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SYMBOL", "invalid symbol")
		return
	}

	tfStr := r.URL.Query().Get("timeframe")
	if tfStr == "" {
		tfStr = "4h"
	}
	if _, err := ParseTimeframe(tfStr); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TIMEFRAME", "invalid timeframe")
		return
	}

	result, err := h.uc.Evaluate(r.Context(), symbolStr, tfStr)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := setupResponse{
		Symbol:          result.Symbol,
		Timeframe:       result.Timeframe,
		BestSetup:       string(result.BestSetup),
		Score:           result.Score,
		Scores:          makeSetupScoresDTO(result.Scores),
		TrendHealth:     result.TrendHealth,
		Regime:          result.Regime,
		MarketEffective: result.MarketEffective,
		Confidence:      result.Confidence,
	}
	writeJSON(w, http.StatusOK, resp)
}

// extractSetupSymbol parses the symbol from paths like /api/token/BTCUSDT/setup.
func extractSetupSymbol(path string) (string, bool) {
	if !strings.HasPrefix(path, setupPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, setupPrefix)
	if !strings.HasSuffix(rest, setupSuffix) {
		return "", false
	}
	symbol := strings.TrimSuffix(rest, setupSuffix)
	if symbol == "" {
		return "", false
	}
	return symbol, true
}

// DTO types for the setup response.
type setupResponse struct {
	Symbol          string             `json:"symbol"`
	Timeframe       string             `json:"timeframe"`
	BestSetup       string             `json:"bestSetup"`
	Score           float64            `json:"score"`
	Scores          map[string]float64 `json:"scores"`
	TrendHealth     float64            `json:"trendHealth"`
	Regime          string             `json:"regime"`
	MarketEffective float64            `json:"marketEffective"`
	Confidence      float64            `json:"confidence"`
}

func makeSetupScoresDTO(scores map[setup.SetupType]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	for k, v := range scores {
		out[string(k)] = v
	}
	return out
}
