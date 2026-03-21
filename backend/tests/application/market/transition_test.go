package market_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/market/transition"
	mkt "pano_chart/backend/domain/market"
)

// ---------- helpers ----------

func approxT(t *testing.T, got, want, delta float64, label string) {
	t.Helper()
	if math.Abs(got-want) > delta {
		t.Errorf("%s: got %.6f, want %.6f (±%v)", label, got, want, delta)
	}
}

// ---------- pressure model ----------

func TestRegimeAgeFactor_boundaries(t *testing.T) {
	tests := []struct {
		age  int
		want float64
	}{
		{0, 0},
		{-5, 0},
		{1, 1.0 / 30},
		{15, 0.5},
		{30, 1},
		{100, 1},
	}
	for _, tc := range tests {
		got := transition.RegimeAgeFactor(tc.age)
		approxT(t, got, tc.want, 1e-9, "regimeAgeFactor")
	}
}

func TestBreakoutPressure_clamped(t *testing.T) {
	// With very high inputs the result must be clamped at 1.
	got := transition.BreakoutPressure(1.0, 2.0, 60)
	if got > 1.0 {
		t.Errorf("breakoutPressure should be clamped to 1, got %f", got)
	}
}

func TestBreakoutPressure_basicFormula(t *testing.T) {
	// compressionBreadth=0.5, volSlope=0.2, age=15 → factor=0.5
	// expected = 0.5 * (1+0.2) * 0.5 = 0.3
	got := transition.BreakoutPressure(0.5, 0.2, 15)
	approxT(t, got, 0.3, 1e-9, "pressure")
}

func TestVolatilitySlope_emptySlice(t *testing.T) {
	got := transition.VolatilitySlope(nil)
	if got != 0 {
		t.Errorf("expected 0 for nil slice, got %f", got)
	}
}

func TestVolatilitySlope_normalCase(t *testing.T) {
	// (1.5 - 1.0) / 1.0 = 0.5
	got := transition.VolatilitySlope([]float64{1.0, 1.2, 1.5})
	approxT(t, got, 0.5, 1e-9, "volSlope")
}

// ---------- transition engine ----------

func TestEngine_FromCompression_zeroPressure(t *testing.T) {
	eng := transition.NewTransitionEngine()
	// compressionBreadth=0 → pressure=0 → sideways=1
	p := eng.Calculate(mkt.RegimeCompression, 0, 0, 0)
	approxT(t, p.Trend, 0, 1e-9, "trend")
	approxT(t, p.Expansion, 0, 1e-9, "expansion")
	approxT(t, p.Sideways, 1.0, 1e-9, "sideways")
}

func TestEngine_FromCompression_highPressure(t *testing.T) {
	eng := transition.NewTransitionEngine()
	// compressionBreadth=1.0, volSlope=0.5, age=30 → factor=1
	// pressure = 1.0 * 1.5 * 1 = 1.5 → clamped to 1
	// trendP = 0.6, expansionP = 0.4, sidewaysP = 0
	p := eng.Calculate(mkt.RegimeCompression, 1.0, 0.5, 30)
	approxT(t, p.Trend, 0.6, 1e-9, "trend")
	approxT(t, p.Expansion, 0.4, 1e-9, "expansion")
	approxT(t, p.Sideways, 0.0, 1e-9, "sideways")
}

func TestEngine_FromTrend_zeroPressure(t *testing.T) {
	eng := transition.NewTransitionEngine()
	p := eng.Calculate(mkt.RegimeTrend, 0, 0, 0)
	approxT(t, p.Trend, 0.6, 1e-9, "trend")
	approxT(t, p.Expansion, 0.2, 1e-9, "expansion")
	approxT(t, p.Sideways, 0.2, 1e-9, "sideways")
}

func TestEngine_FromTrend_highPressure(t *testing.T) {
	eng := transition.NewTransitionEngine()
	// pressure clamped to 1 → trend=0, expansion=0.5, sideways=0.5
	p := eng.Calculate(mkt.RegimeTrend, 1.0, 0.5, 30)
	approxT(t, p.Trend, 0.0, 1e-9, "trend")
	approxT(t, p.Expansion, 0.5, 1e-9, "expansion")
	approxT(t, p.Sideways, 0.5, 1e-9, "sideways")
}

func TestEngine_FromExpansion_fixedProbabilities(t *testing.T) {
	eng := transition.NewTransitionEngine()
	p := eng.Calculate(mkt.RegimeExpansion, 0.5, 0.5, 30)
	approxT(t, p.Trend, 0.25, 1e-9, "trend")
	approxT(t, p.Sideways, 0.55, 1e-9, "sideways")
	approxT(t, p.Expansion, 0.20, 1e-9, "expansion")
}

func TestEngine_FromSideways_lowBreadth(t *testing.T) {
	eng := transition.NewTransitionEngine()
	p := eng.Calculate(mkt.RegimeSideways, 0.1, 0, 0)
	// pressureShift=0.05, trendP=0.23, expansionP=0.12, sidewaysP=0.65
	approxT(t, p.Trend, 0.23, 1e-9, "trend")
	approxT(t, p.Expansion, 0.12, 1e-9, "expansion")
	approxT(t, p.Sideways, 0.65, 1e-9, "sideways")
}

func TestEngine_FromSideways_highBreadth(t *testing.T) {
	eng := transition.NewTransitionEngine()
	p := eng.Calculate(mkt.RegimeSideways, 1.0, 0, 0)
	// pressureShift=0.5, trendP=0.5, expansionP=0.3, sidewaysP=0.2
	approxT(t, p.Trend, 0.5, 1e-9, "trend")
	approxT(t, p.Expansion, 0.3, 1e-9, "expansion")
	approxT(t, p.Sideways, 0.2, 1e-9, "sideways")
}

func TestEngine_probabilities_sum_to_one(t *testing.T) {
	eng := transition.NewTransitionEngine()
	cases := []struct {
		regime mkt.Regime
		cb     float64
		vs     float64
		age    int
	}{
		{mkt.RegimeCompression, 0.3, 0.1, 10},
		{mkt.RegimeTrend, 0.5, 0.3, 20},
		{mkt.RegimeExpansion, 0.8, 0.5, 30},
		{mkt.RegimeSideways, 0.2, 0.0, 5},
	}
	for _, tc := range cases {
		p := eng.Calculate(tc.regime, tc.cb, tc.vs, tc.age)
		sum := p.Trend + p.Sideways + p.Expansion
		approxT(t, sum, 1.0, 1e-6, string(tc.regime)+" sum")
	}
}

// ---------- transition service ----------

type fakeTransitionRegimeProvider struct {
	summary mkt.RegimeSummary
	err     error
}

func (f *fakeTransitionRegimeProvider) CalculateRegime(_ context.Context, tf string) (mkt.RegimeSummary, error) {
	if f.err != nil {
		return mkt.RegimeSummary{}, f.err
	}
	s := f.summary
	s.Timeframe = tf
	return s, nil
}

func TestTransitionService_Calculate(t *testing.T) {
	provider := &fakeTransitionRegimeProvider{
		summary: mkt.RegimeSummary{
			Regime:     mkt.RegimeCompression,
			Prevalence: 0.8,
			Metrics: mkt.RegimeMetrics{
				CompressionBreadth:  0.4,
				VolatilityExpansion: 1.2, // volSlope = 0.2
			},
		},
	}
	eng := transition.NewTransitionEngine()
	svc := transition.NewTransitionService(provider, eng)

	result, err := svc.Calculate(context.Background(), "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Timeframe != "4h" {
		t.Errorf("timeframe: got %q, want %q", result.Timeframe, "4h")
	}
	if result.CurrentRegime != mkt.RegimeCompression {
		t.Errorf("regime: got %q, want %q", result.CurrentRegime, mkt.RegimeCompression)
	}
	if result.Horizon != "12 candles (~2d)" {
		t.Errorf("horizon: got %q, want %q", result.Horizon, "12 candles (~2d)")
	}

	// Verify probabilities sum to 1.
	sum := result.Probabilities.Trend + result.Probabilities.Sideways + result.Probabilities.Expansion
	approxT(t, sum, 1.0, 1e-6, "probability sum")

	// Verify some pressure was applied (trend + expansion > 0).
	if result.Probabilities.Trend <= 0 {
		t.Error("expected positive trend probability")
	}
}

func TestTransitionService_propagatesError(t *testing.T) {
	provider := &fakeTransitionRegimeProvider{
		err: context.DeadlineExceeded,
	}
	eng := transition.NewTransitionEngine()
	svc := transition.NewTransitionService(provider, eng)

	_, err := svc.Calculate(context.Background(), "4h")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- HTTP handler ----------

type fakeTransitionCalculator struct {
	result mkt.MarketTransition
	err    error
}

func (f *fakeTransitionCalculator) Calculate(_ context.Context, tf string) (mkt.MarketTransition, error) {
	if f.err != nil {
		return mkt.MarketTransition{}, f.err
	}
	r := f.result
	r.Timeframe = tf
	return r, nil
}

func TestMarketTransitionHandler_success(t *testing.T) {
	calc := &fakeTransitionCalculator{
		result: mkt.MarketTransition{
			CurrentRegime: mkt.RegimeCompression,
			Probabilities: mkt.TransitionProbabilities{
				Trend:     0.42,
				Sideways:  0.28,
				Expansion: 0.30,
			},
			Horizon: "12 candles (~2d)",
		},
	}
	handler := adhttp.NewMarketTransitionHandler(calc)

	req := httptest.NewRequest("GET", "/api/market/transition?timeframe=1h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Timeframe     string `json:"timeframe"`
		CurrentRegime string `json:"currentRegime"`
		Probabilities struct {
			Trend     float64 `json:"trend"`
			Sideways  float64 `json:"sideways"`
			Expansion float64 `json:"expansion"`
		} `json:"probabilities"`
		Horizon string `json:"horizon"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Timeframe != "1h" {
		t.Errorf("timeframe: got %q, want %q", resp.Timeframe, "1h")
	}
	if resp.CurrentRegime != "compression" {
		t.Errorf("regime: got %q", resp.CurrentRegime)
	}
	if resp.Probabilities.Trend != 0.42 {
		t.Errorf("trend: got %f, want 0.42", resp.Probabilities.Trend)
	}
	if resp.Horizon != "12 candles (~2d)" {
		t.Errorf("horizon: got %q", resp.Horizon)
	}
}

func TestMarketTransitionHandler_defaultTimeframe(t *testing.T) {
	calc := &fakeTransitionCalculator{
		result: mkt.MarketTransition{
			CurrentRegime: mkt.RegimeSideways,
			Probabilities: mkt.TransitionProbabilities{Trend: 0.2, Sideways: 0.7, Expansion: 0.1},
			Horizon:       "12 candles (~2d)",
		},
	}
	handler := adhttp.NewMarketTransitionHandler(calc)

	req := httptest.NewRequest("GET", "/api/market/transition", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Timeframe string `json:"timeframe"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Timeframe != "4h" {
		t.Errorf("default timeframe: got %q, want %q", resp.Timeframe, "4h")
	}
}

func TestMarketTransitionHandler_error(t *testing.T) {
	calc := &fakeTransitionCalculator{err: context.DeadlineExceeded}
	handler := adhttp.NewMarketTransitionHandler(calc)

	req := httptest.NewRequest("GET", "/api/market/transition", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
