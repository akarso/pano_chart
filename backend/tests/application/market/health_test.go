package market_test

import (
	"math"
	"testing"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// ---------- ComputeTrendHealth ----------

func TestTrendHealth_Uptrend_PriceAtHigh(t *testing.T) {
	h := appmarket.ComputeTrendHealth("uptrend", 100, 100, 80, 5, 0.5)
	if h != 1.0 {
		t.Errorf("expected 1.0, got %f", h)
	}
}

func TestTrendHealth_Uptrend_OneATRDrawdown(t *testing.T) {
	h := appmarket.ComputeTrendHealth("uptrend", 100, 105, 80, 5, 0.5)
	if h != 0 {
		t.Errorf("expected 0, got %f", h)
	}
}

func TestTrendHealth_Uptrend_HalfATRDrawdown(t *testing.T) {
	h := appmarket.ComputeTrendHealth("uptrend", 100, 102.5, 80, 5, 0.5)
	if h < 0.49 || h > 0.51 {
		t.Errorf("expected ~0.5, got %f", h)
	}
}

func TestTrendHealth_Downtrend_PriceAtLow(t *testing.T) {
	h := appmarket.ComputeTrendHealth("downtrend", 80, 100, 80, 5, -0.5)
	if h != 1.0 {
		t.Errorf("expected 1.0, got %f", h)
	}
}

func TestTrendHealth_Downtrend_OneATRBounce(t *testing.T) {
	h := appmarket.ComputeTrendHealth("downtrend", 85, 100, 80, 5, -0.5)
	if h != 0 {
		t.Errorf("expected 0, got %f", h)
	}
}

func TestTrendHealth_CrashPenalty(t *testing.T) {
	h := appmarket.ComputeTrendHealth("uptrend", 100, 100, 80, 5, -2.0)
	if h < 0.29 || h > 0.31 {
		t.Errorf("expected ~0.3, got %f", h)
	}
}

func TestTrendHealth_ZeroATR(t *testing.T) {
	h := appmarket.ComputeTrendHealth("uptrend", 100, 100, 80, 0, 0.5)
	if h != 0 {
		t.Errorf("expected 0 for zero ATR, got %f", h)
	}
}

func TestTrendHealth_Sideways_ReturnsZero(t *testing.T) {
	h := appmarket.ComputeTrendHealth("sideways", 100, 100, 80, 5, 0.5)
	if h != 0 {
		t.Errorf("expected 0 for non-trend state, got %f", h)
	}
}

// ---------- BuildMarketLabel ----------

func TestBuildMarketLabel_StrongTrend(t *testing.T) {
	l := appmarket.BuildMarketLabel(0.7, 0.6)
	if l != "Strong trend" {
		t.Errorf("expected 'Strong trend', got %q", l)
	}
}

func TestBuildMarketLabel_TrendWeakening(t *testing.T) {
	l := appmarket.BuildMarketLabel(0.7, 0.4)
	if l != "Trend weakening" {
		t.Errorf("expected 'Trend weakening', got %q", l)
	}
}

func TestBuildMarketLabel_TrendBreakingDown(t *testing.T) {
	l := appmarket.BuildMarketLabel(0.7, 0.2)
	if l != "Trend breaking down" {
		t.Errorf("expected 'Trend breaking down', got %q", l)
	}
}

func TestBuildMarketLabel_MixedConditions(t *testing.T) {
	l := appmarket.BuildMarketLabel(0.5, 0.6)
	if l != "Mixed conditions" {
		t.Errorf("expected 'Mixed conditions', got %q", l)
	}
}

func TestBuildMarketLabel_NoClearTrend(t *testing.T) {
	l := appmarket.BuildMarketLabel(0.3, 0.6)
	if l != "No clear trend" {
		t.Errorf("expected 'No clear trend', got %q", l)
	}
}

// ---------- Summary with health fields ----------

func TestCalculate_HealthFieldsPopulated(t *testing.T) {
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{
				TrendScore:   0.8,
				Bias:         "up",
				Price:        100,
				RecentHigh:   100,
				RecentLow:    80,
				ATR:          5,
				RecentReturn: 0.5,
			},
			{
				TrendScore:   0.8,
				Bias:         "up",
				Price:        98,
				RecentHigh:   102,
				RecentLow:    80,
				ATR:          5,
				RecentReturn: 0.5,
			},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.EffectiveTrend <= 0 {
		t.Errorf("expected positive EffectiveTrend, got %f", s.EffectiveTrend)
	}
	if s.Label == "" {
		t.Error("expected non-empty Label")
	}
}

func TestCalculate_NoPriceData_StillWorks(t *testing.T) {
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.8},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.EffectiveTrend != 0 {
		t.Errorf("expected 0 EffectiveTrend without price data, got %f", s.EffectiveTrend)
	}
	// Without ATR data, no tokens contribute to health — breakdownRate is 0
	// (unknown, not "all broken").
	if s.BreakdownRate != 0 {
		t.Errorf("expected breakdownRate 0 without price data, got %f", s.BreakdownRate)
	}
}

func TestCalculate_EmptyEvals_ZeroHealthFields(t *testing.T) {
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.EffectiveTrend != 0 || s.BreakdownRate != 0 {
		t.Errorf("expected zero health fields for empty, got et=%f bd=%f",
			s.EffectiveTrend, s.BreakdownRate)
	}
	if s.Label != "No clear trend" {
		t.Errorf("expected 'No clear trend' for empty, got %q", s.Label)
	}
}

func TestCalculate_BreakdownRate(t *testing.T) {
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{
				// Healthy uptrend: price at recent high.
				TrendScore:   0.9,
				Bias:         "up",
				Price:        100,
				RecentHigh:   100,
				RecentLow:    80,
				ATR:          5,
				RecentReturn: 0.5,
			},
			{
				// Breaking uptrend: price far from high.
				TrendScore:   0.9,
				Bias:         "up",
				Price:        90,
				RecentHigh:   100,
				RecentLow:    80,
				ATR:          5,
				RecentReturn: -2.0, // drawdown = (100-90)/5 = 2.0 ATR → health = 0
			},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	// 1 healthy token, 1 breaking token → 50% breakdown rate.
	if s.BreakdownRate != 0.5 {
		t.Errorf("expected breakdownRate 0.5, got %f", s.BreakdownRate)
	}
}

// ---------- DampenTrendByHealth ----------

func TestDampenTrendByHealth_HealthyTrend_MinimalDampening(t *testing.T) {
	b := mkt.Breadth{Trend: 0.80, Sideways: 0.10, Compression: 0.05, Breakout: 0.05}
	result := appmarket.DampenTrendByHealth(b, 0.8, 0.1) // healthy market
	// High effectiveTrend → dampFactor near 1.0, minimal reduction.
	if result.Trend < 0.70 {
		t.Errorf("expected Trend >= 0.70 for healthy market, got %f", result.Trend)
	}
	sum := result.Trend + result.Sideways + result.Compression + result.Breakout
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("expected breadth sum ≈ 1.0, got %f", sum)
	}
}

func TestDampenTrendByHealth_BreakingTrend_StrongDampening(t *testing.T) {
	b := mkt.Breadth{Trend: 0.80, Sideways: 0.10, Compression: 0.05, Breakout: 0.05}
	result := appmarket.DampenTrendByHealth(b, 0.1, 0.9) // breaking market
	// Low health + high breakdowns → aggressive dampening.
	if result.Trend >= 0.30 {
		t.Errorf("expected Trend < 0.30 for breaking market, got %f", result.Trend)
	}
	sum := result.Trend + result.Sideways + result.Compression + result.Breakout
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("expected breadth sum ≈ 1.0, got %f", sum)
	}
}

func TestDampenTrendByHealth_ZeroHealth_FloorApplied(t *testing.T) {
	b := mkt.Breadth{Trend: 0.90, Sideways: 0.05, Compression: 0.03, Breakout: 0.02}
	result := appmarket.DampenTrendByHealth(b, 0.0, 1.0) // total breakdown
	// dampFactor floored at 0.1.
	if result.Trend < 0.08 || result.Trend > 0.10 {
		t.Errorf("expected Trend near 0.09 (floor), got %f", result.Trend)
	}
}

func TestDampenTrendByHealth_RedistributesToOthers(t *testing.T) {
	b := mkt.Breadth{Trend: 0.60, Sideways: 0.20, Compression: 0.10, Breakout: 0.10}
	result := appmarket.DampenTrendByHealth(b, 0.2, 0.8) // bad health
	// Lost trend weight should go to other regimes proportionally.
	if result.Sideways <= 0.20 {
		t.Errorf("expected Sideways to increase, got %f", result.Sideways)
	}
	if result.Compression <= 0.10 {
		t.Errorf("expected Compression to increase, got %f", result.Compression)
	}
}

// ---------- Bias Override ----------

func TestBiasOverride_NegativeReturnWithUpBias(t *testing.T) {
	// All tokens trending down (negative RecentReturn) but individual biases
	// are weakly "up" — aggregate return wins.
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.5, Bias: "up", RecentReturn: -2.0, ATR: 1, Price: 98, RecentHigh: 100, RecentLow: 95},
			{TrendScore: 0.5, Bias: "up", RecentReturn: -1.5, ATR: 1, Price: 97, RecentHigh: 100, RecentLow: 95},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	// avgReturn = -1.75 (in ATR units) → well below -0.5 threshold.
	if s.Bias == "up" {
		t.Errorf("expected bias != up when aggregate return is strongly negative, got %s", s.Bias)
	}
}

func TestBiasOverride_PositiveReturnWithDownBias(t *testing.T) {
	provider := &fakeEvalProvider{
		evals: []domain.EvaluationSnapshot{
			{TrendScore: 0.5, Bias: "down", RecentReturn: 2.0, ATR: 1, Price: 100, RecentHigh: 100, RecentLow: 95},
			{TrendScore: 0.5, Bias: "down", RecentReturn: 1.5, ATR: 1, Price: 100, RecentHigh: 100, RecentLow: 95},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.Bias == "down" {
		t.Errorf("expected bias != down when aggregate return is strongly positive, got %s", s.Bias)
	}
}

// ---------- Full Integration: Breaking Trend Declassification ----------

func TestCalculate_BreakingTrend_DeclassifiedFromTrend(t *testing.T) {
	// Scenario: all tokens have moderate TrendScore but trends are breaking
	// (price far from highs, large drawdowns).  The market should NOT be
	// classified as "trend" — health dampening should push it elsewhere.
	evals := make([]domain.EvaluationSnapshot, 10)
	for i := range evals {
		evals[i] = domain.EvaluationSnapshot{
			TrendScore:    0.5,
			SidewaysScore: 0.2,
			Bias:          "up",
			Price:         85, // far below recent high
			RecentHigh:    100,
			RecentLow:     80,
			ATR:           5,
			RecentReturn:  -3.0, // large drawdown
		}
	}
	provider := &fakeEvalProvider{evals: evals}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("15m")
	if err != nil {
		t.Fatal(err)
	}
	// With severe breakdowns, trend should be dampened enough that
	// another regime takes over.
	if s.State == mkt.StateTrend {
		t.Errorf("breaking trend should not classify as trend, got state=%s confidence=%f",
			s.State, s.Confidence)
	}
	if s.Bias == "up" {
		t.Errorf("bias should not be up when aggregate return is -3.0 ATR, got %s", s.Bias)
	}
}
