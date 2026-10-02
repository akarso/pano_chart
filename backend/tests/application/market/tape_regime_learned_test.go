package market_test

import (
	"testing"

	appmarket "pano_chart/backend/application/market"
	mkt "pano_chart/backend/domain/market"
	"pano_chart/backend/domain/scoring"
	fixtures "pano_chart/backend/tests/fixtures/candles"
)

func TestScoreMarketTape_FallsBackWhenModelAbsent(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	series := fixtures.Load(t, "messy_uptrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "test")
	if tape.State != mkt.StateTrend {
		t.Fatalf("state=%q want trend (heuristic)", tape.State)
	}
}

func TestScoreMarketTape_PlaceholderModelKeepsHeuristic(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	scoring.SetRegimeModel(&scoring.Model{
		Type:        "logistic",
		Placeholder: true,
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: 5},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: -8},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: -8},
		},
	})

	series := fixtures.Load(t, "messy_uptrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "test")
	if tape.State != mkt.StateTrend {
		t.Fatalf("placeholder must keep heuristic state=trend, got %q", tape.State)
	}
}

func TestScoreMarketTape_LearnedStructureKeepsTrendGate(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	// Model pushes compression structure, but messy_uptrend has TapeTrend ≥ 0.5
	// so State must remain trend (PR-115 gate).
	scoring.SetRegimeModel(&scoring.Model{
		Type: "logistic",
		Classes: map[string]scoring.ClassParams{
			"trend":       {Weights: map[string]float64{"trend": 1}, Bias: -8},
			"sideways":    {Weights: map[string]float64{"sideways": 1}, Bias: -8},
			"compression": {Weights: map[string]float64{"compression": 1}, Bias: 5},
			"expansion":   {Weights: map[string]float64{"expansion": 1}, Bias: -8},
		},
	})

	series := fixtures.Load(t, "messy_uptrend")
	tape := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "test")
	if tape.TrendScore < 0.5 {
		t.Fatalf("fixture TapeTrend=%.3f want ≥0.5", tape.TrendScore)
	}
	if tape.State != mkt.StateTrend {
		t.Fatalf("state=%q want trend (gate), structure=%+v", tape.State, tape.Structure)
	}
	if tape.Structure.Compression <= tape.Structure.Trend {
		t.Fatalf("learned structure should favor compression: %+v", tape.Structure)
	}
}

func TestScoreMarketTape_FallsBackWhenModelHasNoClasses(t *testing.T) {
	scoring.ClearRegimeModel()
	t.Cleanup(scoring.ClearRegimeModel)

	series := fixtures.Load(t, "messy_uptrend")
	heuristic := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "test")

	scoring.SetRegimeModel(&scoring.Model{
		Type:    "logistic",
		Weights: map[string]float64{"trend": 1},
		Bias:    0,
	})
	got := appmarket.ScoreMarketTape(series, series.Timeframe().String(), "test")
	if got.State != heuristic.State {
		t.Fatalf("state=%q want heuristic %q", got.State, heuristic.State)
	}
}
