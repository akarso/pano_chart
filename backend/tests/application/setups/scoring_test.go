package setups_test

import (
	"math"
	"testing"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/domain/setup"
)

// --- TrendHealthModifier tests ---

func TestTrendHealthModifier_HighHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.9)
	if got != 1.05 {
		t.Errorf("expected 1.05, got %f", got)
	}
}

func TestTrendHealthModifier_GoodHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.7)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestTrendHealthModifier_MediumHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.5)
	if got != 0.7 {
		t.Errorf("expected 0.7, got %f", got)
	}
}

func TestTrendHealthModifier_LowHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.3)
	if got != 0.4 {
		t.Errorf("expected 0.4, got %f", got)
	}
}

func TestTrendHealthModifier_VeryLowHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.1)
	if got != 0.2 {
		t.Errorf("expected 0.2, got %f", got)
	}
}

func TestTrendHealthModifier_ZeroHealth(t *testing.T) {
	got := setups.TrendHealthModifier(0.0)
	if got != 0.2 {
		t.Errorf("expected 0.2, got %f", got)
	}
}

func TestTrendHealthModifier_BoundaryAt08(t *testing.T) {
	// 0.8 is not > 0.8, so falls to 1.0
	got := setups.TrendHealthModifier(0.8)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

// --- ApplyContextModifier tests ---

func TestApplyContextModifier_OnlyAdjustsTrend(t *testing.T) {
	raw := setup.SetupScores{
		Symbol: "BTCUSDT",
		Scores: map[setup.SetupType]float64{
			setup.CompressionBreakout: 0.8,
			setup.TrendContinuation:   0.7,
			setup.RangeReversion:      0.5,
		},
		BestSetup: setup.CompressionBreakout,
		Score:     0.8,
	}
	ctx := setups.SetupContext{TrendHealth: 0.5, Regime: "uptrend"}
	result := setups.ApplyContextModifier(raw, ctx)

	// Compression and range must be unchanged.
	if result.Scores[setup.CompressionBreakout] != 0.8 {
		t.Errorf("compression changed: %f", result.Scores[setup.CompressionBreakout])
	}
	if result.Scores[setup.RangeReversion] != 0.5 {
		t.Errorf("range changed: %f", result.Scores[setup.RangeReversion])
	}

	// Trend should be 0.7 * 0.7 = 0.49
	expected := 0.7 * 0.7
	if math.Abs(result.Scores[setup.TrendContinuation]-expected) > 1e-9 {
		t.Errorf("trend score: expected %f, got %f", expected, result.Scores[setup.TrendContinuation])
	}
}

func TestApplyContextModifier_HighHealth_BoostsTrend(t *testing.T) {
	raw := setup.SetupScores{
		Symbol: "ETHUSDT",
		Scores: map[setup.SetupType]float64{
			setup.TrendContinuation: 0.9,
		},
		BestSetup: setup.TrendContinuation,
		Score:     0.9,
	}
	ctx := setups.SetupContext{TrendHealth: 0.85, Regime: "uptrend"}
	result := setups.ApplyContextModifier(raw, ctx)

	// 0.9 * 1.05 = 0.945
	expected := 0.945
	if math.Abs(result.Scores[setup.TrendContinuation]-expected) > 1e-9 {
		t.Errorf("expected %f, got %f", expected, result.Scores[setup.TrendContinuation])
	}
}

func TestApplyContextModifier_LowHealth_DropsTrendBelowCompression(t *testing.T) {
	raw := setup.SetupScores{
		Symbol: "SOLUSDT",
		Scores: map[setup.SetupType]float64{
			setup.CompressionBreakout: 0.6,
			setup.TrendContinuation:   0.7,
			setup.RangeReversion:      0.3,
		},
		BestSetup: setup.TrendContinuation,
		Score:     0.7,
	}
	// Health 0.3 → modifier 0.4 → trend becomes 0.7*0.4 = 0.28
	ctx := setups.SetupContext{TrendHealth: 0.3, Regime: "uptrend"}
	result := setups.ApplyContextModifier(raw, ctx)

	if result.BestSetup != setup.CompressionBreakout {
		t.Errorf("expected compression as best after penalty, got %s", result.BestSetup)
	}
	if result.Score != 0.6 {
		t.Errorf("expected score 0.6, got %f", result.Score)
	}
}

func TestApplyContextModifier_PreservesMetadata(t *testing.T) {
	raw := setup.SetupScores{
		Symbol:    "XRPUSDT",
		Timeframe: "4h",
		Scores: map[setup.SetupType]float64{
			setup.TrendContinuation: 0.5,
		},
		BestSetup: setup.TrendContinuation,
		Score:     0.5,
	}
	ctx := setups.SetupContext{TrendHealth: 0.9, Regime: "downtrend"}
	result := setups.ApplyContextModifier(raw, ctx)

	if result.Symbol != "XRPUSDT" {
		t.Errorf("symbol lost: %s", result.Symbol)
	}
	if result.Timeframe != "4h" {
		t.Errorf("timeframe lost: %s", result.Timeframe)
	}
	if result.TrendHealth != 0.9 {
		t.Errorf("TrendHealth not set: %f", result.TrendHealth)
	}
	if result.Regime != "downtrend" {
		t.Errorf("Regime not set: %s", result.Regime)
	}
}

func TestApplyContextModifier_ClampsPastOne(t *testing.T) {
	raw := setup.SetupScores{
		Symbol: "AVAXUSDT",
		Scores: map[setup.SetupType]float64{
			setup.TrendContinuation: 0.99,
		},
		BestSetup: setup.TrendContinuation,
		Score:     0.99,
	}
	// 0.99 * 1.05 = 1.0395 → should be clamped to 1.0
	ctx := setups.SetupContext{TrendHealth: 0.9}
	result := setups.ApplyContextModifier(raw, ctx)

	if result.Scores[setup.TrendContinuation] > 1.0 {
		t.Errorf("score should be clamped to 1.0, got %f", result.Scores[setup.TrendContinuation])
	}
}

// --- Engine integration with health ---

func TestEngine_TrendHealthModifiesBestPick(t *testing.T) {
	eng := setups.NewEngine()

	// High trend score but terrible health → compression should win.
	ctx := setups.SetupContext{
		Symbol:           "LINKUSDT",
		CompressionScore: 0.6,
		TrendScore:       0.9,
		RangeScore:       0.1,
		VolumeScore:      0.7,
		TrendHealth:      0.1, // very low → modifier 0.2
		Regime:           "uptrend",
	}
	result := eng.Evaluate(ctx)

	// trend raw ≈ 0.9*0.6 + 0.7*0.3 = 0.75; after *0.2 = 0.15
	// compression raw should beat 0.15
	if result.BestSetup == setup.TrendContinuation {
		t.Errorf("trend should not win with very low health, got best=%s", result.BestSetup)
	}
}

func TestEngine_HealthyTrendPreservesBest(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{
		Symbol:           "BTCUSDT",
		CompressionScore: 0.3,
		TrendScore:       0.9,
		RangeScore:       0.1,
		VolumeScore:      0.7,
		TrendHealth:      0.9, // high → modifier 1.05
		Regime:           "uptrend",
	}
	result := eng.Evaluate(ctx)

	if result.BestSetup != setup.TrendContinuation {
		t.Errorf("trend should win with healthy trend, got best=%s", result.BestSetup)
	}
	if result.TrendHealth != 0.9 {
		t.Errorf("TrendHealth not propagated: %f", result.TrendHealth)
	}
}
