package market_test

import (
	"testing"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
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
	if s.BreakdownRate != 1.0 {
		t.Errorf("expected breakdownRate 1.0, got %f", s.BreakdownRate)
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
				TrendScore:   0.9,
				Bias:         "up",
				Price:        100,
				RecentHigh:   100,
				RecentLow:    80,
				ATR:          5,
				RecentReturn: 0.5,
			},
			{
				TrendScore:   0.9,
				Bias:         "up",
				Price:        100,
				RecentHigh:   100,
				RecentLow:    80,
				ATR:          0,
				RecentReturn: 0.5,
			},
		},
	}
	svc := appmarket.NewMarketStateService(provider)
	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatal(err)
	}
	if s.BreakdownRate != 0.5 {
		t.Errorf("expected breakdownRate 0.5, got %f", s.BreakdownRate)
	}
}
