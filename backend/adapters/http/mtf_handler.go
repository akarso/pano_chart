package http

import (
	"context"
	"net/http"
	"strings"

	"pano_chart/backend/application/mtf"
)

// mtfSuffix is the action suffix this handler serves under symbolDetailPrefix
// (defined in symbol_detail_handler.go): GET /api/symbol/{symbol}/regimes.
const mtfSuffix = "/regimes"

// MTFCalculator defines the boundary the handler depends on.
type MTFCalculator interface {
	Calculate(ctx context.Context, symbol string) (mtf.Stack, error)
}

// MTFHandler handles GET /api/symbol/{symbol}/regimes.
type MTFHandler struct {
	calc MTFCalculator
}

// NewMTFHandler constructs the handler.
func NewMTFHandler(calc MTFCalculator) *MTFHandler {
	return &MTFHandler{calc: calc}
}

// ServeHTTP implements http.Handler.
func (h *MTFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}

	symbolStr, ok := extractMTFSymbol(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /api/symbol/{symbol}/regimes")
		return
	}

	sym, err := ParseSymbol(symbolStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SYMBOL", "invalid symbol")
		return
	}

	stack, err := h.calc.Calculate(r.Context(), sym.String())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, mtfStackDTOFrom(stack))
}

// extractMTFSymbol parses the symbol from paths like /api/symbol/BTCUSDT/regimes.
func extractMTFSymbol(path string) (string, bool) {
	if !strings.HasPrefix(path, symbolDetailPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, symbolDetailPrefix)
	if !strings.HasSuffix(rest, mtfSuffix) {
		return "", false
	}
	symbol := strings.TrimSuffix(rest, mtfSuffix)
	if symbol == "" {
		return "", false
	}
	return symbol, true
}

// DTO types for the regimes response.
type mtfStackDTO struct {
	Symbol       string        `json:"symbol"`
	Frames       []mtfFrameDTO `json:"frames"`
	Alignment    float64       `json:"alignment"`
	AlignedState string        `json:"alignedState"`
}

type mtfFrameDTO struct {
	Timeframe string          `json:"timeframe"`
	Structure mtfStructureDTO `json:"structure"`
	Dominant  string          `json:"dominant"`
	Bias      string          `json:"bias"`
	Score     float64         `json:"score"`
}

type mtfStructureDTO struct {
	Trend       float64 `json:"trend"`
	Sideways    float64 `json:"sideways"`
	Compression float64 `json:"compression"`
	Expansion   float64 `json:"expansion"`
}

func mtfStackDTOFrom(stack mtf.Stack) mtfStackDTO {
	frames := make([]mtfFrameDTO, len(stack.Frames))
	for i, f := range stack.Frames {
		frames[i] = mtfFrameDTO{
			Timeframe: f.Timeframe,
			Structure: mtfStructureDTO{
				Trend:       f.Structure.Trend,
				Sideways:    f.Structure.Sideways,
				Compression: f.Structure.Compression,
				Expansion:   f.Structure.Expansion,
			},
			Dominant: string(f.Dominant),
			Bias:     f.Bias,
			Score:    f.Score,
		}
	}
	return mtfStackDTO{
		Symbol:       stack.Symbol,
		Frames:       frames,
		Alignment:    stack.Alignment,
		AlignedState: string(stack.AlignedState),
	}
}
