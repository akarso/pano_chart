package behavior_test

import (
	"testing"

	appbehavior "pano_chart/backend/application/behavior"
)

const epsilon = 1e-9

// --- Engine.Evaluate tests ---

func TestEvaluate_HighGreed(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FundingExtremeness: 0.9,
		Imbalance:          0.9,
		OIExpansion:        0.8,
		FragilityScore:     0.1,
		Volatility:         0.1,
		VolumeScore:        0.1,
		Regime:             "trending_up",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	if r.Greed < 0.5 {
		t.Errorf("expected high greed, got %f", r.Greed)
	}
	if r.Fear > r.Greed {
		t.Errorf("fear (%f) should not exceed greed (%f) in greedy context", r.Fear, r.Greed)
	}
}

func TestEvaluate_HighFear(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     0.9,
		Volatility:         0.9,
		FundingExtremeness: 0.1,
		Imbalance:          0.1,
		OIExpansion:        0.1,
		VolumeScore:        0.1,
		Regime:             "trending_down",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	if r.Fear < 0.3 {
		t.Errorf("expected elevated fear, got %f", r.Fear)
	}
}

func TestEvaluate_HighPanicFromFragilityAndVolatility(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     0.95,
		Volatility:         0.95,
		FundingExtremeness: 0.1,
		Imbalance:          0.1,
		OIExpansion:        0.1,
		VolumeScore:        0.1,
		Regime:             "range",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	if r.Panic < 0.3 {
		t.Errorf("expected elevated panic, got %f", r.Panic)
	}
}

func TestEvaluate_PatienceInCompression(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		Volatility:         0.1,
		VolumeScore:        0.1,
		Regime:             "compression",
		FragilityScore:     0.1,
		FundingExtremeness: 0.1,
		Imbalance:          0.1,
		OIExpansion:        0.1,
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	if r.Patience < 0.4 {
		t.Errorf("expected elevated patience in compression, got %f", r.Patience)
	}
}

func TestEvaluate_NeutralContext(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     0.3,
		FundingExtremeness: 0.3,
		OIExpansion:        0.3,
		Imbalance:          0.3,
		Volatility:         0.3,
		VolumeScore:        0.3,
		Regime:             "range",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	if r.Summary != "Neutral sentiment" {
		t.Errorf("expected 'Neutral sentiment', got '%s'", r.Summary)
	}
}

func TestEvaluate_AllZero(t *testing.T) {
	ctx := appbehavior.BehaviorContext{}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	// With zero inputs, patience should be elevated (1-0)*0.5 + (1-0)*0.2 = 0.7
	if r.Patience < 0.4 {
		t.Errorf("expected some patience with zero inputs, got %f", r.Patience)
	}
}

func TestEvaluate_AllOne(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     1.0,
		FundingExtremeness: 1.0,
		OIExpansion:        1.0,
		Imbalance:          1.0,
		Volatility:         1.0,
		VolumeScore:        1.0,
		Regime:             "compression",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	total := r.Greed + r.Fear + r.Patience + r.Panic
	if total > 1.5+epsilon {
		t.Errorf("normalization failed: total %f > 1.5", total)
	}
}

// --- Normalization tests ---

func TestNormalization_CapsTotal(t *testing.T) {
	// Context that would produce very high raw scores.
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     1.0,
		FundingExtremeness: 1.0,
		OIExpansion:        1.0,
		Imbalance:          1.0,
		Volatility:         1.0,
		VolumeScore:        1.0,
		Regime:             "compression",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	total := r.Greed + r.Fear + r.Patience + r.Panic
	if total > 1.5+epsilon {
		t.Errorf("expected total <= 1.5, got %f", total)
	}
}

func TestNormalization_LowTotalUnchanged(t *testing.T) {
	ctx := appbehavior.BehaviorContext{
		FragilityScore:     0.1,
		FundingExtremeness: 0.1,
		OIExpansion:        0.1,
		Imbalance:          0.1,
		Volatility:         0.1,
		VolumeScore:        0.1,
		Regime:             "range",
	}
	e := appbehavior.NewEngine()
	r := e.Evaluate(ctx)

	total := r.Greed + r.Fear + r.Patience + r.Panic
	// Low inputs should not be scaled.
	if total > 1.5 {
		t.Errorf("unexpected normalization for low inputs: total %f", total)
	}
}

// --- Summarize tests ---

func TestSummarize_PanicRising(t *testing.T) {
	s := appbehavior.Summarize(0.0, 0.0, 0.0, 0.8)
	if s != "Panic rising" {
		t.Errorf("expected 'Panic rising', got '%s'", s)
	}
}

func TestSummarize_GreedDominant(t *testing.T) {
	s := appbehavior.Summarize(0.8, 0.0, 0.0, 0.0)
	if s != "Greed dominant" {
		t.Errorf("expected 'Greed dominant', got '%s'", s)
	}
}

func TestSummarize_MarketWaiting(t *testing.T) {
	s := appbehavior.Summarize(0.0, 0.0, 0.7, 0.0)
	if s != "Market waiting / coiling" {
		t.Errorf("expected 'Market waiting / coiling', got '%s'", s)
	}
}

func TestSummarize_FearElevated(t *testing.T) {
	s := appbehavior.Summarize(0.0, 0.7, 0.0, 0.0)
	if s != "Fear elevated" {
		t.Errorf("expected 'Fear elevated', got '%s'", s)
	}
}

func TestSummarize_NeutralSentiment(t *testing.T) {
	s := appbehavior.Summarize(0.3, 0.3, 0.3, 0.3)
	if s != "Neutral sentiment" {
		t.Errorf("expected 'Neutral sentiment', got '%s'", s)
	}
}

func TestSummarize_PriorityOrder_PanicBeatsGreed(t *testing.T) {
	s := appbehavior.Summarize(0.8, 0.0, 0.0, 0.8)
	if s != "Panic rising" {
		t.Errorf("expected 'Panic rising' (highest priority), got '%s'", s)
	}
}

func TestSummarize_PriorityOrder_GreedBeatsPatience(t *testing.T) {
	s := appbehavior.Summarize(0.8, 0.0, 0.7, 0.0)
	if s != "Greed dominant" {
		t.Errorf("expected 'Greed dominant', got '%s'", s)
	}
}

// --- Clamp tests ---

func TestEvaluate_ValuesInRange(t *testing.T) {
	cases := []appbehavior.BehaviorContext{
		{FragilityScore: 2, Volatility: 2, FundingExtremeness: 2, Imbalance: 2, OIExpansion: 2, VolumeScore: 2},
		{FragilityScore: -1, Volatility: -1, FundingExtremeness: -1, Imbalance: -1, OIExpansion: -1, VolumeScore: -1},
	}
	e := appbehavior.NewEngine()
	for _, ctx := range cases {
		r := e.Evaluate(ctx)
		for _, v := range []float64{r.Greed, r.Fear, r.Patience, r.Panic} {
			if v < 0 || v > 1 {
				t.Errorf("value out of [0,1]: %f", v)
			}
		}
	}
}

// --- Symbol / Timeframe populated ---

func TestEvaluate_DoesNotSetSymbolTimeframe(t *testing.T) {
	e := appbehavior.NewEngine()
	r := e.Evaluate(appbehavior.BehaviorContext{})
	if r.Symbol != "" || r.Timeframe != "" {
		t.Error("engine should not set symbol or timeframe")
	}
}
