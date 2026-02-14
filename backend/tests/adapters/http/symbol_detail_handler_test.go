package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpAdapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// --- Fake use case (stub) ---

type fakeGetSymbolDetail struct {
	result usecases.SymbolDetailResult
	err    error
}

func (f *fakeGetSymbolDetail) Execute(_ context.Context, req usecases.GetSymbolDetailRequest) (usecases.SymbolDetailResult, error) {
	if f.err != nil {
		return usecases.SymbolDetailResult{}, f.err
	}
	return f.result, nil
}

// --- Helper to build a valid use case stub ---

func newFakeUC(result usecases.SymbolDetailResult, err error) *usecases.GetSymbolDetail {
	fake := &fakeGetSymbolDetail{result: result, err: err}
	// Provide nil or zero values for other dependencies as fakes for testing
	return usecases.NewGetSymbolDetail(
		fake,         // CandleRepositoryPort
		nil,          // SymbolScorer
		nil,          // SymbolUniverseProvider
		"",           // exchange string
		"",           // venue string
		0,            // lookback int
		0,            // minCandles int
	)
}

// --- Error response DTO for assertions ---

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Tests ---

func TestSymbolDetailHandler_MissingSymbol(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "INVALID_SYMBOL" {
		t.Errorf("expected code INVALID_SYMBOL, got %s", resp.Error.Code)
	}
}

func TestSymbolDetailHandler_InvalidPrefix(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/other/BTCUSDT?timeframe=1h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected code NOT_FOUND, got %s", resp.Error.Code)
	}
}

func TestSymbolDetailHandler_MissingTimeframe(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "INVALID_TIMEFRAME" {
		t.Errorf("expected code INVALID_TIMEFRAME, got %s", resp.Error.Code)
	}
	if resp.Error.Message != "missing timeframe" {
		t.Errorf("expected message 'missing timeframe', got %s", resp.Error.Message)
	}
}

func TestSymbolDetailHandler_InvalidTimeframe(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "INVALID_TIMEFRAME" {
		t.Errorf("expected code INVALID_TIMEFRAME, got %s", resp.Error.Code)
	}
}

func TestSymbolDetailHandler_SymbolNotFound(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, usecases.ErrSymbolNotFound))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "INVALID_SYMBOL" {
		t.Errorf("expected code INVALID_SYMBOL, got %s", resp.Error.Code)
	}
	if resp.Error.Message != "symbol not found" {
		t.Errorf("expected message 'symbol not found', got %s", resp.Error.Message)
	}
}

func TestSymbolDetailHandler_InternalError(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, errGeneric))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %s", resp.Error.Code)
	}
}

var errGeneric = fmt.Errorf("something broke")

func TestSymbolDetailHandler_SuccessWithCandles(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("1h")

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candle, _ := domain.NewCandle(sym, tf, ts, 100.0, 110.0, 90.0, 105.0, 1000.0)

	result := usecases.SymbolDetailResult{
		Symbol:  sym,
		Candles: []domain.Candle{candle},
		Stats: &usecases.SymbolStats{
			TotalScore: 85.5,
			Scores:     map[string]float64{"momentum": 90.0, "volume": 81.0},
		},
	}

	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(result, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["symbol"] != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %v", resp["symbol"])
	}
	if resp["timeframe"] != "1h" {
		t.Errorf("expected timeframe 1h, got %v", resp["timeframe"])
	}

	candles, ok := resp["candles"].([]interface{})
	if !ok {
		t.Fatal("expected candles to be an array")
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles))
	}

	c := candles[0].(map[string]interface{})
	if c["openTime"] != "2024-01-01T00:00:00Z" {
		t.Errorf("expected openTime 2024-01-01T00:00:00Z, got %v", c["openTime"])
	}
	if c["open"].(float64) != 100.0 {
		t.Errorf("expected open 100, got %v", c["open"])
	}
	if c["high"].(float64) != 110.0 {
		t.Errorf("expected high 110, got %v", c["high"])
	}
	if c["low"].(float64) != 90.0 {
		t.Errorf("expected low 90, got %v", c["low"])
	}
	if c["close"].(float64) != 105.0 {
		t.Errorf("expected close 105, got %v", c["close"])
	}
	if c["volume"].(float64) != 1000.0 {
		t.Errorf("expected volume 1000, got %v", c["volume"])
	}

	stats, ok := resp["stats"].(map[string]interface{})
	if !ok {
		t.Fatal("expected stats to be an object")
	}
	if stats["totalScore"].(float64) != 85.5 {
		t.Errorf("expected totalScore 85.5, got %v", stats["totalScore"])
	}
}

func TestSymbolDetailHandler_SuccessNilStats(t *testing.T) {
	sym, _ := domain.NewSymbol("ETHUSDT")

	result := usecases.SymbolDetailResult{
		Symbol:  sym,
		Candles: []domain.Candle{},
		Stats:   nil,
	}

	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(result, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/ETHUSDT?timeframe=4h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	stats := resp["stats"].(map[string]interface{})
	if stats["totalScore"].(float64) != 0 {
		t.Errorf("expected totalScore 0 for nil stats, got %v", stats["totalScore"])
	}

	scores := stats["scores"].(map[string]interface{})
	if len(scores) != 0 {
		t.Errorf("expected empty scores for nil stats, got %v", scores)
	}
}

func TestSymbolDetailHandler_LimitParam(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")

	result := usecases.SymbolDetailResult{
		Symbol:  sym,
		Candles: []domain.Candle{},
		Stats:   nil,
	}

	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(result, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h&limit=50", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestSymbolDetailHandler_InvalidLimitDefaultsToZero(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")

	result := usecases.SymbolDetailResult{
		Symbol:  sym,
		Candles: []domain.Candle{},
		Stats:   nil,
	}

	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(result, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h&limit=abc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Invalid limit should not cause an error; it defaults to 0
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

// --- Pure helper tests ---

func TestHasValidPrefix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/symbol/BTCUSDT", true},
		{"/api/symbol/", true},
		{"/api/other/BTCUSDT", false},
		{"/api/symbols/BTCUSDT", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := httpAdapter.HasValidPrefix(tt.path)
			if got != tt.want {
				t.Errorf("HasValidPrefix(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractSymbol(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"valid symbol", "/api/symbol/BTCUSDT", "BTCUSDT", false},
		{"empty symbol", "/api/symbol/", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := httpAdapter.ExtractSymbol(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractSymbol(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractSymbol(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"100", 100},
		{"abc", 0},
		{"0", 0},
		{"-1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := httpAdapter.ParseLimit(tt.input)
			if got != tt.want {
				t.Errorf("ParseLimit(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSymbolDetailHandler_ResponseContentType(t *testing.T) {
	sym, _ := domain.NewSymbol("BTCUSDT")

	result := usecases.SymbolDetailResult{
		Symbol:  sym,
		Candles: []domain.Candle{},
		Stats:   nil,
	}

	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(result, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT?timeframe=1h", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestSymbolDetailHandler_ErrorResponseContentType(t *testing.T) {
	handler := httpAdapter.NewSymbolDetailHandler(newFakeUC(usecases.SymbolDetailResult{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json on error, got %s", ct)
	}
}
