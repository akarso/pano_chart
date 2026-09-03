package setups_test

import (
	"testing"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/domain/setup"
)

// --- clamp tests ---

func TestClamp_BelowZeroReturnsZero(t *testing.T) {
	ctx := setups.SetupContext{CompressionScore: -1, TrendScore: -1, Volatility: 5}
	eng := setups.NewEngine()
	result := eng.Evaluate(ctx)
	for st, score := range result.Scores {
		if score < 0 || score > 1 {
			t.Errorf("score for %s out of [0,1]: %f", st, score)
		}
	}
}

func TestClamp_AboveOneReturnsCapped(t *testing.T) {
	ctx := setups.SetupContext{CompressionScore: 5, TrendScore: 5, RangeScore: 5, VolumeScore: 5, Volatility: 5}
	eng := setups.NewEngine()
	result := eng.Evaluate(ctx)
	for st, score := range result.Scores {
		if score > 1 {
			t.Errorf("score for %s exceeds 1: %f", st, score)
		}
	}
}

// --- Engine tests ---

func TestEngine_EvaluateReturnsAllThreeSetups(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{
		Symbol:           "BTCUSDT",
		CompressionScore: 0.8,
		TrendScore:       0.6,
		RangeScore:       0.4,
		VolumeScore:      0.5,
		Volatility:       0.3,
	}
	result := eng.Evaluate(ctx)

	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", result.Symbol)
	}
	if len(result.Scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(result.Scores))
	}
	for _, st := range []setup.SetupType{setup.CompressionBreakout, setup.TrendContinuation, setup.RangeReversion} {
		if _, ok := result.Scores[st]; !ok {
			t.Errorf("missing score for %s", st)
		}
	}
}

func TestEngine_BestSetupIsHighestScore(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{
		Symbol:           "ETHUSDT",
		CompressionScore: 0.9,
		TrendScore:       0.1,
		RangeScore:       0.1,
		VolumeScore:      0.5,
		Volatility:       0.2,
	}
	result := eng.Evaluate(ctx)

	// With compression=0.9, low trend/range, compression_breakout should win.
	if result.BestSetup != setup.CompressionBreakout {
		t.Errorf("expected best setup %s, got %s", setup.CompressionBreakout, result.BestSetup)
	}
	if result.Score != result.Scores[result.BestSetup] {
		t.Errorf("Score (%f) != Scores[BestSetup] (%f)", result.Score, result.Scores[result.BestSetup])
	}
}

func TestEngine_TrendWinsWhenDominant(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{
		Symbol:           "SOLUSDT",
		CompressionScore: 0.1,
		TrendScore:       0.95,
		RangeScore:       0.1,
		VolumeScore:      0.8,
		Volatility:       0.1,
		TrendHealth:      0.9,
		Regime:           "uptrend",
	}
	result := eng.Evaluate(ctx)

	if result.BestSetup != setup.TrendContinuation {
		t.Errorf("expected trend_continuation as best, got %s", result.BestSetup)
	}
}

func TestEngine_RangeWinsWhenDominant(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{
		Symbol:     "XRPUSDT",
		RangeScore: 0.95,
		Volatility: 0.9,
		// Keep others low
		CompressionScore: 0.0,
		TrendScore:       0.0,
		VolumeScore:      0.0,
	}
	result := eng.Evaluate(ctx)

	if result.BestSetup != setup.RangeReversion {
		t.Errorf("expected range_reversion as best, got %s", result.BestSetup)
	}
}

func TestEngine_ZeroContextProducesLowScores(t *testing.T) {
	eng := setups.NewEngine()
	ctx := setups.SetupContext{Symbol: "EMPTY"}
	result := eng.Evaluate(ctx)

	// With all-zero input, compression_breakout = (1-0)*0.3 = 0.3,
	// trend_continuation = (1-0)*0.1 = 0.1, range_reversion = 0.
	for st, score := range result.Scores {
		if score < 0 || score > 1 {
			t.Errorf("score for %s out of range: %f", st, score)
		}
	}
	// range_reversion should always be 0 with zero context
	if result.Scores[setup.RangeReversion] != 0 {
		t.Errorf("expected 0 for range_reversion, got %f", result.Scores[setup.RangeReversion])
	}
}

// --- Individual evaluator tests ---

func TestCompressionSetup_HighCompressionLowVolatility(t *testing.T) {
	cs := setups.CompressionSetup{}
	score := cs.Score(setups.SetupContext{
		CompressionScore: 0.9,
		Volatility:       0.1,
		VolumeScore:      0.7,
	})
	// compression*0.5 + (1-volatility)*0.3 + volume*0.2
	// 0.9*0.5 + 0.9*0.3 + 0.7*0.2 = 0.45 + 0.27 + 0.14 = 0.86
	expected := 0.86
	if diff := score - expected; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected ~%.2f, got %.4f", expected, score)
	}
}

func TestTrendSetup_HighTrendHighVolume(t *testing.T) {
	ts := setups.TrendSetup{}
	score := ts.Score(setups.SetupContext{
		TrendScore:  0.8,
		VolumeScore: 0.9,
		Volatility:  0.2,
	})
	// trend*0.6 + volume*0.3 + (1-volatility)*0.1
	// 0.8*0.6 + 0.9*0.3 + 0.8*0.1 = 0.48 + 0.27 + 0.08 = 0.83
	expected := 0.83
	if diff := score - expected; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected ~%.2f, got %.4f", expected, score)
	}
}

func TestRangeSetup_HighRangeHighVolatility(t *testing.T) {
	rs := setups.RangeSetup{}
	score := rs.Score(setups.SetupContext{
		RangeScore: 0.8,
		Volatility: 0.7,
	})
	// range*0.7 + volatility*0.3
	// 0.8*0.7 + 0.7*0.3 = 0.56 + 0.21 = 0.77
	expected := 0.77
	if diff := score - expected; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected ~%.2f, got %.4f", expected, score)
	}
}

func TestAllScoresInRange(t *testing.T) {
	eng := setups.NewEngine()
	cases := []setups.SetupContext{
		{CompressionScore: 1, TrendScore: 1, RangeScore: 1, VolumeScore: 1, Volatility: 1},
		{CompressionScore: 0, TrendScore: 0, RangeScore: 0, VolumeScore: 0, Volatility: 0},
		{CompressionScore: 0.5, TrendScore: 0.5, RangeScore: 0.5, VolumeScore: 0.5, Volatility: 0.5},
	}
	for _, ctx := range cases {
		result := eng.Evaluate(ctx)
		for st, score := range result.Scores {
			if score < 0 || score > 1 {
				t.Errorf("score for %s out of range [0,1]: %f", st, score)
			}
		}
	}
}
