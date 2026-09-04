package market_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	adhttp "pano_chart/backend/adapters/http"
	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake implementations ---
//
// PR-073: /api/market/regime is now backed by the same MarketStateService
// that serves /api/market/state — both handlers just consume mkt.Summary
// via the same "Calculate(timeframe) (mkt.Summary, error)" shape.

type fakeRegimeCalc struct {
	summary mkt.Summary
	err     error
}

func (f *fakeRegimeCalc) CalculateWithCandleMetrics(_ context.Context, _ string) (mkt.Summary, error) {
	return f.summary, f.err
}

func TestRegimeHandler_DefaultParams(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.Summary{
			Timeframe:  "4h",
			State:      mkt.StateCompression,
			Confidence: 0.71,
			Breadth: mkt.Breadth{
				Expansion:   0.05,
				Compression: 0.71,
				Trend:       0.14,
				Sideways:    0.10,
			},
			VolatilityExpansion: 0.82,
			Dispersion:          0.21,
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Timeframe  string  `json:"timeframe"`
		Regime     string  `json:"regime"`
		Prevalence float64 `json:"prevalence"`
		Scores     struct {
			Expansion   float64 `json:"expansion"`
			Compression float64 `json:"compression"`
			Trend       float64 `json:"trend"`
			Sideways    float64 `json:"sideways"`
		} `json:"scores"`
		Metrics struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			SidewaysBreadth     float64 `json:"sidewaysBreadth"`
			ExpansionBreadth    float64 `json:"expansionBreadth"`
			CompressionBreadth  float64 `json:"compressionBreadth"`
			VolatilityExpansion float64 `json:"volatilityExpansion"`
			Dispersion          float64 `json:"dispersion"`
		} `json:"metrics"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", resp.Timeframe)
	}
	if resp.Regime != "compression" {
		t.Errorf("expected regime compression, got %s", resp.Regime)
	}
	if resp.Prevalence != 0.71 {
		t.Errorf("expected prevalence 0.71, got %f", resp.Prevalence)
	}
	if resp.Scores.Compression != 0.71 {
		t.Errorf("expected scores.compression 0.71, got %f", resp.Scores.Compression)
	}
	// Metrics' breadth fields must mirror the same Breadth used in "scores" —
	// there's only one breadth computation now (see PR-073).
	if resp.Metrics.TrendBreadth != 0.14 {
		t.Errorf("expected trendBreadth 0.14, got %f", resp.Metrics.TrendBreadth)
	}
	if resp.Metrics.CompressionBreadth != 0.71 {
		t.Errorf("expected compressionBreadth 0.71, got %f", resp.Metrics.CompressionBreadth)
	}
	// These non-null fields are required by the Flutter client's fromJson
	// (frontend/lib/features/market_state/regime_data.dart) — dropping them
	// would crash the app, not just change a number.
	if resp.Metrics.VolatilityExpansion != 0.82 {
		t.Errorf("expected volatilityExpansion 0.82, got %f", resp.Metrics.VolatilityExpansion)
	}
	if resp.Metrics.Dispersion != 0.21 {
		t.Errorf("expected dispersion 0.21, got %f", resp.Metrics.Dispersion)
	}
}

func TestRegimeHandler_CustomTimeframe(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.Summary{
			Timeframe: "1h",
			State:     mkt.StateSideways,
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Timeframe string `json:"timeframe"`
		Regime    string `json:"regime"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", resp.Timeframe)
	}
}

func TestRegimeHandler_Error(t *testing.T) {
	calc := &fakeRegimeCalc{err: fmt.Errorf("something went wrong")}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestRegimeHandler_RoundsValues(t *testing.T) {
	calc := &fakeRegimeCalc{
		summary: mkt.Summary{
			Timeframe:  "4h",
			State:      mkt.StateTrend,
			Confidence: 0.123456789,
			Breadth: mkt.Breadth{
				Expansion:   0.111111111,
				Compression: 0.222222222,
				Trend:       0.444444444,
				Sideways:    0.222222222,
			},
			VolatilityExpansion: 1.555555555,
			Dispersion:          0.333333333,
		},
	}

	handler := adhttp.NewMarketRegimeHandler(calc)
	req := httptest.NewRequest(http.MethodGet, "/api/market/regime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Prevalence float64 `json:"prevalence"`
		Scores     struct {
			Trend float64 `json:"trend"`
		} `json:"scores"`
		Metrics struct {
			TrendBreadth        float64 `json:"trendBreadth"`
			VolatilityExpansion float64 `json:"volatilityExpansion"`
			Dispersion          float64 `json:"dispersion"`
		} `json:"metrics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	check := func(name string, got, want float64) {
		if math.Abs(got-want) > 0.00001 {
			t.Errorf("%s: expected %f, got %f", name, want, got)
		}
	}
	check("prevalence", resp.Prevalence, 0.1235)
	check("scores.trend", resp.Scores.Trend, 0.4444)
	check("trendBreadth", resp.Metrics.TrendBreadth, 0.4444)
	check("volatilityExpansion", resp.Metrics.VolatilityExpansion, 1.5556)
	check("dispersion", resp.Metrics.Dispersion, 0.3333)
}

// TestRegimeAndStateHandlers_AgreeOnSameSummary is the regression test PR-073
// set out to make possible: /api/market/regime and /api/market/state are now
// both thin views over the same *appmarket.MarketStateService, so they can
// no longer disagree about the dominant regime/state or bias for the same
// underlying data — which two independently-evolved pipelines previously
// could (this repo's original PR-073 motivation).
func TestRegimeAndStateHandlers_AgreeOnSameSummary(t *testing.T) {
	// A clearly trend-dominant, bullish set of evaluations.
	evals := []domain.EvaluationSnapshot{
		{TrendScore: 0.9, SidewaysScore: 0.1, CompressionScore: 0.05, Bias: "up", RecentReturn: 1.0},
		{TrendScore: 0.85, SidewaysScore: 0.15, CompressionScore: 0.05, Bias: "up", RecentReturn: 0.8},
	}
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: evals})

	regimeHandler := adhttp.NewMarketRegimeHandler(svc)
	stateHandler := adhttp.NewMarketHandler(svc)

	regimeRec := httptest.NewRecorder()
	regimeHandler.ServeHTTP(regimeRec, httptest.NewRequest(http.MethodGet, "/api/market/regime", nil))

	stateRec := httptest.NewRecorder()
	stateHandler.ServeHTTP(stateRec, httptest.NewRequest(http.MethodGet, "/api/market/state", nil))

	var regimeResp struct {
		Regime string `json:"regime"`
		Bias   string `json:"bias"`
	}
	var stateResp struct {
		State string `json:"state"`
		Bias  string `json:"bias"`
	}
	if err := json.NewDecoder(regimeRec.Body).Decode(&regimeResp); err != nil {
		t.Fatalf("decode regime response: %v", err)
	}
	if err := json.NewDecoder(stateRec.Body).Decode(&stateResp); err != nil {
		t.Fatalf("decode state response: %v", err)
	}

	if regimeResp.Regime != stateResp.State {
		t.Errorf("regime/state disagree: regime=%q state=%q", regimeResp.Regime, stateResp.State)
	}
	if regimeResp.Bias != stateResp.Bias {
		t.Errorf("bias disagrees between endpoints: regime=%q state=%q", regimeResp.Bias, stateResp.Bias)
	}
}
