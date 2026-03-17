package http

import (
	"context"
	"net/http"
	"strings"

	domainrisk "pano_chart/backend/domain/risk"
)

const fragilityPrefix = "/api/token/"
const fragilitySuffix = "/fragility"

// FragilityUseCase defines the boundary the handler depends on.
type FragilityUseCase interface {
	Get(ctx context.Context, symbol, timeframe string) (domainrisk.Fragility, error)
}

// FragilityHandler handles GET /api/token/{symbol}/fragility requests.
type FragilityHandler struct {
	uc FragilityUseCase
}

// NewFragilityHandler constructs the handler.
func NewFragilityHandler(uc FragilityUseCase) *FragilityHandler {
	return &FragilityHandler{uc: uc}
}

// ServeHTTP implements http.Handler.
func (h *FragilityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}

	symbolStr, ok := extractFragilitySymbol(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /api/token/{symbol}/fragility")
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

	resp := fragilityResponse{
		Symbol:       result.Symbol,
		Timeframe:    result.Timeframe,
		Score:        result.Score,
		RiskLevel:    result.RiskLevel,
		DominantSide: result.DominantSide,
		SqueezeRisk:  result.SqueezeRisk,
		Components: fragilityComponentsDTO{
			FundingExtremeness:   result.Components.FundingExtremeness,
			OIExpansion:          result.Components.OIExpansion,
			LongShortImbalance:   result.Components.LongShortImbalance,
			LiquidationProximity: result.Components.LiquidationProximity,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// extractFragilitySymbol parses the symbol from paths like /api/token/BTCUSDT/fragility.
func extractFragilitySymbol(path string) (string, bool) {
	if !strings.HasPrefix(path, fragilityPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, fragilityPrefix)
	if !strings.HasSuffix(rest, fragilitySuffix) {
		return "", false
	}
	symbol := strings.TrimSuffix(rest, fragilitySuffix)
	if symbol == "" {
		return "", false
	}
	return symbol, true
}

// DTO types for the fragility response.
type fragilityResponse struct {
	Symbol       string                 `json:"symbol"`
	Timeframe    string                 `json:"timeframe"`
	Score        float64                `json:"fragilityScore"`
	RiskLevel    string                 `json:"riskLevel"`
	DominantSide string                 `json:"dominantSide"`
	SqueezeRisk  string                 `json:"squeezeRisk"`
	Components   fragilityComponentsDTO `json:"components"`
}

type fragilityComponentsDTO struct {
	FundingExtremeness   float64 `json:"fundingExtremeness"`
	OIExpansion          float64 `json:"oiExpansion"`
	LongShortImbalance   float64 `json:"longShortImbalance"`
	LiquidationProximity float64 `json:"liquidationProximity"`
}
