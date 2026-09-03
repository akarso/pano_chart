package http

import (
	"context"
	"net/http"
	"strings"

	domainbehavior "pano_chart/backend/domain/behavior"
)

const behaviorPrefix = "/api/token/"
const behaviorSuffix = "/behavior"

// BehaviorUseCase defines the boundary the handler depends on.
type BehaviorUseCase interface {
	Get(ctx context.Context, symbol, timeframe string) (domainbehavior.RetailBehavior, error)
}

// BehaviorHandler handles GET /api/token/{symbol}/behavior requests.
type BehaviorHandler struct {
	uc BehaviorUseCase
}

// NewBehaviorHandler constructs the handler.
func NewBehaviorHandler(uc BehaviorUseCase) *BehaviorHandler {
	return &BehaviorHandler{uc: uc}
}

// ServeHTTP implements http.Handler.
func (h *BehaviorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}

	symbolStr, ok := extractBehaviorSymbol(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /api/token/{symbol}/behavior")
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

	result, err := h.uc.Get(r.Context(), symbolStr, tfStr)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := behaviorResponse{
		Symbol:    result.Symbol,
		Timeframe: result.Timeframe,
		Greed:     result.Greed,
		Fear:      result.Fear,
		Patience:  result.Patience,
		Panic:     result.Panic,
		Summary:   result.Summary,
	}
	writeJSON(w, http.StatusOK, resp)
}

// extractBehaviorSymbol parses the symbol from paths like /api/token/BTCUSDT/behavior.
func extractBehaviorSymbol(path string) (string, bool) {
	if !strings.HasPrefix(path, behaviorPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, behaviorPrefix)
	if !strings.HasSuffix(rest, behaviorSuffix) {
		return "", false
	}
	symbol := strings.TrimSuffix(rest, behaviorSuffix)
	if symbol == "" {
		return "", false
	}
	return symbol, true
}

// DTO types for the behavior response.
type behaviorResponse struct {
	Symbol    string  `json:"symbol"`
	Timeframe string  `json:"timeframe"`
	Greed     float64 `json:"greed"`
	Fear      float64 `json:"fear"`
	Patience  float64 `json:"patience"`
	Panic     float64 `json:"panic"`
	Summary   string  `json:"summary"`
}
