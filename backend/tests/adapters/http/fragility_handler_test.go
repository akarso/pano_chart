package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	httpAdapter "pano_chart/backend/adapters/http"
	domainrisk "pano_chart/backend/domain/risk"
)

// --- Fake use case for fragility handler ---

type fakeFragilityUC struct {
	result domainrisk.Fragility
	err    error
}

func (f *fakeFragilityUC) Get(_ context.Context, symbol, timeframe string) (domainrisk.Fragility, error) {
	if f.err != nil {
		return domainrisk.Fragility{}, f.err
	}
	r := f.result
	r.Symbol = symbol
	r.Timeframe = timeframe
	return r, nil
}

func newFragilityHandler(result domainrisk.Fragility, err error) http.Handler {
	return httpAdapter.NewFragilityHandler(&fakeFragilityUC{result: result, err: err})
}

// --- Fragility response DTO ---

type fragilityResp struct {
	Symbol       string  `json:"symbol"`
	Timeframe    string  `json:"timeframe"`
	Score        float64 `json:"fragilityScore"`
	RiskLevel    string  `json:"riskLevel"`
	DominantSide string  `json:"dominantSide"`
	SqueezeRisk  string  `json:"squeezeRisk"`
	Components   struct {
		FundingExtremeness   float64 `json:"fundingExtremeness"`
		OIExpansion          float64 `json:"oiExpansion"`
		LongShortImbalance   float64 `json:"longShortImbalance"`
		LiquidationProximity float64 `json:"liquidationProximity"`
	} `json:"components"`
}

// --- Tests ---

func TestFragilityHandler_HappyPath(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{
		Score:        0.575,
		RiskLevel:    "medium",
		DominantSide: "long",
		SqueezeRisk:  "long_squeeze",
		Components: domainrisk.FragilityComponents{
			FundingExtremeness:   0.5,
			OIExpansion:          0.8,
			LongShortImbalance:   0.3,
			LiquidationProximity: 0.6,
		},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/fragility?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp fragilityResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", resp.Symbol)
	}
	if resp.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", resp.Timeframe)
	}
	if resp.Score != 0.575 {
		t.Errorf("expected score 0.575, got %f", resp.Score)
	}
	if resp.RiskLevel != "medium" {
		t.Errorf("expected riskLevel medium, got %s", resp.RiskLevel)
	}
	if resp.Components.FundingExtremeness != 0.5 {
		t.Errorf("expected fundingExtremeness 0.5, got %f", resp.Components.FundingExtremeness)
	}
	if resp.Components.OIExpansion != 0.8 {
		t.Errorf("expected oiExpansion 0.8, got %f", resp.Components.OIExpansion)
	}
	if resp.Components.LongShortImbalance != 0.3 {
		t.Errorf("expected longShortImbalance 0.3, got %f", resp.Components.LongShortImbalance)
	}
	if resp.Components.LiquidationProximity != 0.6 {
		t.Errorf("expected liquidationProximity 0.6, got %f", resp.Components.LiquidationProximity)
	}
	if resp.DominantSide != "long" {
		t.Errorf("expected dominantSide long, got %s", resp.DominantSide)
	}
	if resp.SqueezeRisk != "long_squeeze" {
		t.Errorf("expected squeezeRisk long_squeeze, got %s", resp.SqueezeRisk)
	}
}

func TestFragilityHandler_DefaultTimeframe(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{
		Score:     0.3,
		RiskLevel: "low",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/ETHUSDT/fragility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp fragilityResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Timeframe != "4h" {
		t.Errorf("expected default timeframe 4h, got %s", resp.Timeframe)
	}
}

func TestFragilityHandler_InvalidPath_NoSymbol(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token//fragility", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFragilityHandler_InvalidPath_NoFragilitySuffix(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFragilityHandler_InvalidTimeframe(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/fragility?timeframe=invalid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFragilityHandler_MethodNotAllowed(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/token/BTCUSDT/fragility?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFragilityHandler_UseCaseError(t *testing.T) {
	handler := newFragilityHandler(domainrisk.Fragility{}, errors.New("data provider down"))

	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/fragility?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- Token router tests ---

func TestTokenRouter_RoutesToSetup(t *testing.T) {
	setupCalled := false
	setup := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setupCalled = true
		w.WriteHeader(http.StatusOK)
	})
	fragility := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("fragility handler should not be called")
	})

	router := httpAdapter.NewTokenRouter(setup, fragility)
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/setup", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !setupCalled {
		t.Error("setup handler was not called")
	}
}

func TestTokenRouter_RoutesToFragility(t *testing.T) {
	fragilityCalled := false
	setup := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("setup handler should not be called")
	})
	fragility := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fragilityCalled = true
		w.WriteHeader(http.StatusOK)
	})

	router := httpAdapter.NewTokenRouter(setup, fragility)
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/fragility", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !fragilityCalled {
		t.Error("fragility handler was not called")
	}
}

func TestTokenRouter_UnknownEndpoint_Returns404(t *testing.T) {
	setup := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("setup handler should not be called")
	})
	fragility := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("fragility handler should not be called")
	})

	router := httpAdapter.NewTokenRouter(setup, fragility)
	req := httptest.NewRequest(http.MethodGet, "/api/token/BTCUSDT/unknown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
