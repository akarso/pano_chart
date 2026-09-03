package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

const symbolDetailPrefix = "/api/symbol/"

// SymbolDetailUseCase defines the use-case boundary for the handler.
type SymbolDetailUseCase interface {
	Execute(ctx context.Context, req usecases.GetSymbolDetailRequest) (usecases.SymbolDetailResult, error)
}

// SymbolDetailHandler handles GET /api/symbol/{symbol} requests.
type SymbolDetailHandler struct {
	uc SymbolDetailUseCase
}

// NewSymbolDetailHandler constructs the handler.
func NewSymbolDetailHandler(uc SymbolDetailUseCase) *SymbolDetailHandler {
	return &SymbolDetailHandler{uc: uc}
}

// ServeHTTP implements http.Handler.
func (h *SymbolDetailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := h.buildRequest(r)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	result, err := h.executeUseCase(r.Context(), req)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := buildResponse(result, req.TimeframeStr)
	writeJSON(w, http.StatusOK, resp)
}

// symbolDetailHTTPRequest holds validated request data.
type symbolDetailHTTPRequest struct {
	Symbol       domain.Symbol
	Timeframe    domain.Timeframe
	TimeframeStr string
	Limit        int
}

// buildRequest validates the request and builds the DTO.
func (h *SymbolDetailHandler) buildRequest(r *http.Request) (*symbolDetailHTTPRequest, error) {
	if !HasValidPrefix(r.URL.Path) {
		return nil, newBadRequestError("NOT_FOUND", "endpoint not found")
	}

	symbolStr, err := ExtractSymbol(r.URL.Path)
	if err != nil {
		return nil, newBadRequestError("INVALID_SYMBOL", err.Error())
	}

	symbol, err := ParseSymbol(symbolStr)
	if err != nil {
		return nil, newBadRequestError("INVALID_SYMBOL", "invalid symbol")
	}

	tfStr := r.URL.Query().Get("timeframe")
	if tfStr == "" {
		return nil, newBadRequestError("INVALID_TIMEFRAME", "missing timeframe")
	}

	tf, err := ParseTimeframe(tfStr)
	if err != nil {
		return nil, newBadRequestError("INVALID_TIMEFRAME", "invalid timeframe")
	}

	limit := ParseLimit(r.URL.Query().Get("limit"))

	return &symbolDetailHTTPRequest{
		Symbol:       symbol,
		Timeframe:    tf,
		TimeframeStr: tfStr,
		Limit:        limit,
	}, nil
}

// executeUseCase runs the business logic.
func (h *SymbolDetailHandler) executeUseCase(ctx context.Context, req *symbolDetailHTTPRequest) (usecases.SymbolDetailResult, error) {
	result, err := h.uc.Execute(ctx, usecases.GetSymbolDetailRequest{
		Symbol:    req.Symbol,
		Timeframe: req.Timeframe,
		Limit:     req.Limit,
	})
	if err != nil {
		if errors.Is(err, usecases.ErrSymbolNotFound) {
			return usecases.SymbolDetailResult{}, newNotFoundError("INVALID_SYMBOL", "symbol not found")
		}
		return usecases.SymbolDetailResult{}, newInternalError("INTERNAL_ERROR", "internal error")
	}
	return result, nil
}

// HTTP errors for domain-level error handling.
type httpError struct {
	Status  int
	Code    string
	Message string
}

func (e *httpError) Error() string {
	return e.Message
}

func newBadRequestError(code, msg string) *httpError {
	return &httpError{
		Status:  http.StatusBadRequest,
		Code:    code,
		Message: msg,
	}
}

func newNotFoundError(code, msg string) *httpError {
	return &httpError{
		Status:  http.StatusNotFound,
		Code:    code,
		Message: msg,
	}
}

func newInternalError(code, msg string) *httpError {
	return &httpError{
		Status:  http.StatusInternalServerError,
		Code:    code,
		Message: msg,
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		writeError(w, httpErr.Status, httpErr.Code, httpErr.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
}

// DTO types (unchanged).
type symbolDetailResponse struct {
	Symbol    string            `json:"symbol"`
	Timeframe string            `json:"timeframe"`
	Candles   []symbolCandleDTO `json:"candles"`
	Stats     symbolStatsDTO    `json:"stats"`
}

type symbolCandleDTO struct {
	OpenTime string  `json:"openTime"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

type symbolStatsDTO struct {
	TotalScore float64            `json:"totalScore"`
	Scores     map[string]float64 `json:"scores"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Pure helpers (mostly unchanged).
func HasValidPrefix(path string) bool {
	return strings.HasPrefix(path, symbolDetailPrefix)
}

func ExtractSymbol(path string) (string, error) {
	symbolStr := strings.TrimPrefix(path, symbolDetailPrefix)
	if symbolStr == "" {
		return "", errors.New("symbol is required")
	}
	return symbolStr, nil
}

func ParseSymbol(s string) (domain.Symbol, error) {
	return domain.NewSymbol(s)
}

func ParseTimeframe(s string) (domain.Timeframe, error) {
	return domain.NewTimeframe(s)
}

func ParseLimit(limitStr string) int {
	if limitStr == "" {
		return 0
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0
	}
	return limit
}

func buildResponse(result usecases.SymbolDetailResult, tfStr string) symbolDetailResponse {
	candles := buildCandlesDTOs(result.Candles)
	stats := buildStatsDTO(result.Stats)

	return symbolDetailResponse{
		Symbol:    result.Symbol.String(),
		Timeframe: tfStr,
		Candles:   candles,
		Stats:     stats,
	}
}

func buildCandlesDTOs(candles []domain.Candle) []symbolCandleDTO {
	dtos := make([]symbolCandleDTO, 0, len(candles))
	for _, c := range candles {
		dtos = append(dtos, symbolCandleDTO{
			OpenTime: c.Timestamp().UTC().Format(time.RFC3339),
			Open:     c.Open(),
			High:     c.High(),
			Low:      c.Low(),
			Close:    c.Close(),
			Volume:   c.Volume(),
		})
	}
	return dtos
}

func buildStatsDTO(stats *usecases.SymbolStats) symbolStatsDTO {
	if stats == nil {
		return symbolStatsDTO{TotalScore: 0, Scores: map[string]float64{}}
	}
	return symbolStatsDTO{
		TotalScore: stats.TotalScore,
		Scores:     stats.Scores,
	}
}

// HTTP writers (unchanged).
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: errorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
