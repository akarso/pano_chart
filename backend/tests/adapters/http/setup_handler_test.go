package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/domain/setup"
)

// --- Fake use case for setup handler ---

type fakeSetupUC struct {
	result setup.SetupScores
	err    error
}

func (f *fakeSetupUC) Evaluate(_ context.Context, symbol, timeframe string) (setup.SetupScores, error) {
	if f.err != nil {
		return setup.SetupScores{}, f.err
	}
	r := f.result
	r.Symbol = symbol
	r.Timeframe = timeframe
	return r, nil
}

func newSetupHandler(result setup.SetupScores, err error) http.Handler {
	return httpAdapter.NewSetupHandler(&fakeSetupUC{result: result, err: err})
}

// --- Setup response DTO ---

type setupResp struct {
	Symbol    string             `json:"symbol"`
	Timeframe string             `json:"timeframe"`
	BestSetup string             `json:"bestSetup"`
	Score     float64            `json:"score"`
	Scores    map[string]float64 `json:"scores"`
}

// --- Tests ---

func TestSetupHandler_HappyPath(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{
		BestSetup: setup.CompressionBreakout,
		Score:     0.78,
		Scores: map[setup.SetupType]float64{
			setup.CompressionBreakout: 0.78,
			setup.TrendContinuation:   0.34,
			setup.RangeReversion:      0.22,
		},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/setup?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp setupResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", resp.Symbol)
	}
	if resp.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", resp.Timeframe)
	}
	if resp.BestSetup != "compression_breakout" {
		t.Errorf("expected bestSetup compression_breakout, got %s", resp.BestSetup)
	}
	if resp.Score != 0.78 {
		t.Errorf("expected score 0.78, got %f", resp.Score)
	}
	if len(resp.Scores) != 3 {
		t.Errorf("expected 3 scores, got %d", len(resp.Scores))
	}
}

func TestSetupHandler_DefaultTimeframe(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{
		BestSetup: setup.TrendContinuation,
		Score:     0.5,
		Scores:    map[setup.SetupType]float64{},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/ETHUSDT/setup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp setupResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Timeframe != "4h" {
		t.Errorf("expected default timeframe 4h, got %s", resp.Timeframe)
	}
}

func TestSetupHandler_InvalidPath_NoSymbol(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token//setup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSetupHandler_InvalidPath_NoSetupSuffix(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSetupHandler_InvalidTimeframe(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/setup?timeframe=invalid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSetupHandler_MethodNotAllowed(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/token/BTCUSDT/setup?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSetupHandler_UseCaseError(t *testing.T) {
	handler := newSetupHandler(setup.SetupScores{}, errors.New("something broke"))

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/setup?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
