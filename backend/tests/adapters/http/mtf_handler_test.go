package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/mtf"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake calculator for the /regimes handler ---

type fakeMTFCalculator struct {
	stack mtf.Stack
	err   error
}

func (f *fakeMTFCalculator) Calculate(context.Context, string) (mtf.Stack, error) {
	if f.err != nil {
		return mtf.Stack{}, f.err
	}
	return f.stack, nil
}

func newMTFHandler(stack mtf.Stack, err error) http.Handler {
	return httpAdapter.NewMTFHandler(&fakeMTFCalculator{stack: stack, err: err})
}

type mtfStackResp struct {
	Symbol    string  `json:"symbol"`
	Alignment float64 `json:"alignment"`
	Aligned   string  `json:"alignedState"`
	Frames    []struct {
		Timeframe string  `json:"timeframe"`
		Dominant  string  `json:"dominant"`
		Bias      string  `json:"bias"`
		Score     float64 `json:"score"`
		Structure struct {
			Trend       float64 `json:"trend"`
			Sideways    float64 `json:"sideways"`
			Compression float64 `json:"compression"`
			Expansion   float64 `json:"expansion"`
		} `json:"structure"`
	} `json:"frames"`
}

func TestMTFHandler_HappyPath(t *testing.T) {
	stack := mtf.Stack{
		Symbol: "BTCUSDT",
		Frames: []mtf.TFRegime{
			{
				Timeframe: "15m",
				Structure: mkt.Breadth{Trend: 0.8, Sideways: 0.1, Compression: 0.05, Expansion: 0.05},
				Dominant:  mkt.StateTrend,
				Bias:      "up",
				Score:     0.8,
			},
		},
		Alignment:    1.0,
		AlignedState: mkt.StateTrend,
	}
	handler := newMTFHandler(stack, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/regimes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp mtfStackResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", resp.Symbol)
	}
	if resp.Alignment != 1.0 {
		t.Errorf("expected alignment 1.0, got %g", resp.Alignment)
	}
	if resp.Aligned != "trend" {
		t.Errorf("expected alignedState trend, got %s", resp.Aligned)
	}
	if len(resp.Frames) != 1 || resp.Frames[0].Dominant != "trend" {
		t.Fatalf("unexpected frames: %+v", resp.Frames)
	}
	if resp.Frames[0].Structure.Trend != 0.8 {
		t.Errorf("expected structure.trend 0.8, got %g", resp.Frames[0].Structure.Trend)
	}
}

func TestMTFHandler_InvalidPath_NoSymbol(t *testing.T) {
	handler := newMTFHandler(mtf.Stack{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol//regimes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMTFHandler_InvalidPath_NoRegimesSuffix(t *testing.T) {
	handler := newMTFHandler(mtf.Stack{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMTFHandler_InvalidSymbol(t *testing.T) {
	handler := newMTFHandler(mtf.Stack{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/bad!symbol/regimes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMTFHandler_MethodNotAllowed(t *testing.T) {
	handler := newMTFHandler(mtf.Stack{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/symbol/BTCUSDT/regimes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestMTFHandler_CalculatorError(t *testing.T) {
	handler := newMTFHandler(mtf.Stack{}, errors.New("store unavailable"))

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/BTCUSDT/regimes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
