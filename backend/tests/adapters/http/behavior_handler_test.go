package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	domainbehavior "pano_chart/backend/domain/behavior"
)

// --- Fake use case ---

type fakeBehaviorUC struct {
	result domainbehavior.RetailBehavior
	err    error
}

func (f *fakeBehaviorUC) Get(_ context.Context, symbol, timeframe string) (domainbehavior.RetailBehavior, error) {
	if f.err != nil {
		return domainbehavior.RetailBehavior{}, f.err
	}
	r := f.result
	r.Symbol = symbol
	r.Timeframe = timeframe
	return r, nil
}

func newBehaviorHandler(result domainbehavior.RetailBehavior, err error) http.Handler {
	return httpAdapter.NewBehaviorHandler(&fakeBehaviorUC{result: result, err: err})
}

// --- Response DTO for unmarshaling ---

type behaviorResp struct {
	Symbol    string  `json:"symbol"`
	Timeframe string  `json:"timeframe"`
	Greed     float64 `json:"greed"`
	Fear      float64 `json:"fear"`
	Patience  float64 `json:"patience"`
	Panic     float64 `json:"panic"`
	Summary   string  `json:"summary"`
}

// --- Tests ---

func TestBehaviorHandler_HappyPath(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{
		Greed:    0.68,
		Fear:     0.32,
		Patience: 0.51,
		Panic:    0.21,
		Summary:  "Neutral sentiment",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/behavior?timeframe=4h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp behaviorResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", resp.Symbol)
	}
	if resp.Timeframe != "4h" {
		t.Errorf("expected 4h, got %s", resp.Timeframe)
	}
	if resp.Greed != 0.68 {
		t.Errorf("expected greed 0.68, got %f", resp.Greed)
	}
	if resp.Fear != 0.32 {
		t.Errorf("expected fear 0.32, got %f", resp.Fear)
	}
	if resp.Patience != 0.51 {
		t.Errorf("expected patience 0.51, got %f", resp.Patience)
	}
	if resp.Panic != 0.21 {
		t.Errorf("expected panic 0.21, got %f", resp.Panic)
	}
	if resp.Summary != "Neutral sentiment" {
		t.Errorf("expected 'Neutral sentiment', got '%s'", resp.Summary)
	}
}

func TestBehaviorHandler_DefaultTimeframe(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{
		Summary: "Neutral sentiment",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/ETHUSDT/behavior", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp behaviorResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Timeframe != "4h" {
		t.Errorf("expected default timeframe 4h, got %s", resp.Timeframe)
	}
}

func TestBehaviorHandler_MethodNotAllowed(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/token/BTCUSDT/behavior", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestBehaviorHandler_InvalidPath(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/token//behavior", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBehaviorHandler_InvalidTimeframe(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/behavior?timeframe=99x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBehaviorHandler_UseCaseError(t *testing.T) {
	handler := newBehaviorHandler(domainbehavior.RetailBehavior{}, errors.New("internal"))
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/behavior?timeframe=4h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestTokenRouter_RoutesToBehavior(t *testing.T) {
	behaviorCalled := false
	setup := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("setup handler should not be called")
	})
	fragility := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("fragility handler should not be called")
	})
	behavior := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		behaviorCalled = true
		w.WriteHeader(http.StatusOK)
	})

	router := httpAdapter.NewTokenRouter(setup, fragility, behavior)
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/behavior", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !behaviorCalled {
		t.Error("behavior handler was not called")
	}
}
