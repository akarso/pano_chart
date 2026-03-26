package setups_test

import (
	"math"
	"testing"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/domain/setup"
)

// --- MarketModifier: trend regime ---

func TestMarketModifier_TrendRegime_StrongMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "uptrend"}
	got := setups.MarketModifier(s, 0.7)
	if got != 1.1 {
		t.Errorf("expected 1.1, got %f", got)
	}
}

func TestMarketModifier_TrendRegime_ModerateMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "downtrend"}
	got := setups.MarketModifier(s, 0.5)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestMarketModifier_TrendRegime_WeakeningMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "uptrend"}
	got := setups.MarketModifier(s, 0.3)
	if got != 0.7 {
		t.Errorf("expected 0.7, got %f", got)
	}
}

func TestMarketModifier_TrendRegime_BrokenMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "uptrend"}
	got := setups.MarketModifier(s, 0.2)
	if got != 0.4 {
		t.Errorf("expected 0.4, got %f", got)
	}
}

// --- MarketModifier: sideways regime ---

func TestMarketModifier_Sideways_WeakMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "sideways"}
	got := setups.MarketModifier(s, 0.2)
	if got != 1.1 {
		t.Errorf("expected 1.1, got %f", got)
	}
}

func TestMarketModifier_Sideways_ModerateMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "sideways"}
	got := setups.MarketModifier(s, 0.45)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

func TestMarketModifier_Sideways_StrongMarket(t *testing.T) {
	s := setup.SetupScores{Regime: "sideways"}
	got := setups.MarketModifier(s, 0.7)
	if got != 0.8 {
		t.Errorf("expected 0.8, got %f", got)
	}
}

// --- MarketModifier: compression regime ---

func TestMarketModifier_Compression_TransitionZone(t *testing.T) {
	s := setup.SetupScores{Regime: "compression"}
	got := setups.MarketModifier(s, 0.5)
	if got != 1.1 {
		t.Errorf("expected 1.1, got %f", got)
	}
}

func TestMarketModifier_Compression_LowEffective(t *testing.T) {
	s := setup.SetupScores{Regime: "compression"}
	got := setups.MarketModifier(s, 0.2)
	if got != 0.9 {
		t.Errorf("expected 0.9, got %f", got)
	}
}

func TestMarketModifier_Compression_HighEffective(t *testing.T) {
	s := setup.SetupScores{Regime: "compression"}
	got := setups.MarketModifier(s, 0.8)
	if got != 0.9 {
		t.Errorf("expected 0.9, got %f", got)
	}
}

func TestMarketModifier_Compression_MidRange(t *testing.T) {
	s := setup.SetupScores{Regime: "compression"}
	got := setups.MarketModifier(s, 0.35)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

// --- MarketModifier: unknown regime ---

func TestMarketModifier_UnknownRegime_ReturnsOne(t *testing.T) {
	s := setup.SetupScores{Regime: ""}
	got := setups.MarketModifier(s, 0.5)
	if got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

// --- ApplyMarketModifier ---

func TestApplyMarketModifier_ScalesScores(t *testing.T) {
	scores := setup.SetupScores{
		Symbol:    "BTCUSDT",
		Timeframe: "4h",
		BestSetup: setup.TrendContinuation,
		Score:     0.8,
		Scores: map[setup.SetupType]float64{
			setup.CompressionBreakout: 0.5,
			setup.TrendContinuation:   0.8,
			setup.RangeReversion:      0.3,
		},
		Regime:      "uptrend",
		TrendHealth: 0.9,
	}

	result := setups.ApplyMarketModifier(scores, 0.7)

	if math.Abs(result.Score-0.88) > 1e-9 {
		t.Errorf("expected score 0.88, got %f", result.Score)
	}
	if result.MarketEffective != 0.7 {
		t.Errorf("expected MarketEffective 0.7, got %f", result.MarketEffective)
	}
	if result.BestSetup != setup.TrendContinuation {
		t.Errorf("expected trend as best, got %s", result.BestSetup)
	}
}

func TestApplyMarketModifier_WeakMarket_PenalisesTrend(t *testing.T) {
	scores := setup.SetupScores{
		Symbol:    "ETHUSDT",
		BestSetup: setup.TrendContinuation,
		Score:     0.85,
		Scores: map[setup.SetupType]float64{
			setup.TrendContinuation: 0.85,
		},
		Regime: "uptrend",
	}

	result := setups.ApplyMarketModifier(scores, 0.2)

	expected := 0.85 * 0.4
	if math.Abs(result.Score-expected) > 1e-9 {
		t.Errorf("expected score %f, got %f", expected, result.Score)
	}
}

func TestApplyMarketModifier_ClampsAboveOne(t *testing.T) {
	scores := setup.SetupScores{
		Symbol:    "SOLUSDT",
		BestSetup: setup.RangeReversion,
		Score:     0.95,
		Scores: map[setup.SetupType]float64{
			setup.RangeReversion: 0.95,
		},
		Regime: "sideways",
	}

	result := setups.ApplyMarketModifier(scores, 0.1)
	if result.Score > 1.0 {
		t.Errorf("score should be clamped to 1.0, got %f", result.Score)
	}
}

func TestApplyMarketModifier_PreservesMetadata(t *testing.T) {
	scores := setup.SetupScores{
		Symbol:      "XRPUSDT",
		Timeframe:   "1h",
		TrendHealth: 0.6,
		Regime:      "compression",
		BestSetup:   setup.CompressionBreakout,
		Score:       0.7,
		Scores: map[setup.SetupType]float64{
			setup.CompressionBreakout: 0.7,
		},
	}

	result := setups.ApplyMarketModifier(scores, 0.5)

	if result.Symbol != "XRPUSDT" {
		t.Errorf("symbol lost: %s", result.Symbol)
	}
	if result.Timeframe != "1h" {
		t.Errorf("timeframe lost: %s", result.Timeframe)
	}
	if result.TrendHealth != 0.6 {
		t.Errorf("TrendHealth lost: %f", result.TrendHealth)
	}
	if result.Regime != "compression" {
		t.Errorf("Regime lost: %s", result.Regime)
	}
}

// --- MarketLabel ---

func TestMarketLabel_Favorable(t *testing.T) {
	if got := setups.MarketLabel(0.7); got != "Favorable" {
		t.Errorf("expected Favorable, got %s", got)
	}
}

func TestMarketLabel_Neutral(t *testing.T) {
	if got := setups.MarketLabel(0.5); got != "Neutral" {
		t.Errorf("expected Neutral, got %s", got)
	}
}

func TestMarketLabel_Unfavorable(t *testing.T) {
	if got := setups.MarketLabel(0.3); got != "Unfavorable" {
		t.Errorf("expected Unfavorable, got %s", got)
	}
}

// --- Spec example: ranking impact ---

func TestMarketModifier_SpecExample_RankingImpact(t *testing.T) {
	tokenA := setup.SetupScores{
		Symbol:    "A",
		BestSetup: setup.TrendContinuation,
		Score:     0.425,
		Scores:    map[setup.SetupType]float64{setup.TrendContinuation: 0.425},
		Regime:    "uptrend",
	}
	resultA := setups.ApplyMarketModifier(tokenA, 0.25)

	tokenB := setup.SetupScores{
		Symbol:    "B",
		BestSetup: setup.RangeReversion,
		Score:     0.75,
		Scores:    map[setup.SetupType]float64{setup.RangeReversion: 0.75},
		Regime:    "sideways",
	}
	resultB := setups.ApplyMarketModifier(tokenB, 0.25)

	if resultA.Score >= resultB.Score {
		t.Errorf("weak market should rank sideways above trend: A=%f B=%f", resultA.Score, resultB.Score)
	}
}
