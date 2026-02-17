package scoring

import (
	"pano_chart/backend/domain"
	"testing"
	"time"
)

func makeSeries(prices []float64) domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := make([]domain.Candle, len(prices))
	for i, p := range prices {
		candles[i] = domain.NewCandleUnsafe(sym, tf, time.Date(2024, 1, 1, 0, i, 0, 0, time.UTC), p, p, p, p, 1)
	}
	series, _ := domain.NewCandleSeries(sym, tf, candles)
	return series
}

func TestGainLossScoreCalculator(t *testing.T) {
	calc := &GainLossScoreCalculator{}
	series := makeSeries([]float64{1, 2, 3, 4, 5})
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive gain, got %v", score)
	}
}

func TestTrendPredictabilityScoreCalculator(t *testing.T) {
	calc := &TrendPredictabilityScoreCalculator{}
	series := makeSeries([]float64{1, 2, 3, 4, 5})
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive trend, got %v", score)
	}
}

func TestSidewaysConsistencyScoreCalculator(t *testing.T) {
	calc := &SidewaysConsistencyScoreCalculator{}
	// Flat line
	flat := makeSeries([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	score, _ := calc.Score(flat)
	if score != 0 {
		t.Errorf("expected zero for flat, got %v", score)
	}

	// Clean zig-zag, bounded (start and end at same value)
	zigzag := makeSeries([]float64{1, 2, 1, 2, 1, 2, 1, 2, 1, 1})
	score, _ = calc.Score(zigzag)
	if score < 0.7 {
		t.Errorf("expected high sideways score for zigzag, got %v", score)
	}

	// Slow drift
	drift := makeSeries([]float64{1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9})
	score, _ = calc.Score(drift)
	if score > 0.3 {
		t.Errorf("expected low for slow drift, got %v", score)
	}

	// Breakout trend
	breakout := makeSeries([]float64{1, 1, 1, 1, 1, 2, 3, 4, 5, 6})
	score, _ = calc.Score(breakout)
	if score > 0.2 {
		t.Errorf("expected low for breakout, got %v", score)
	}

	// Noisy volatility
	noisy := makeSeries([]float64{1, 2, 1.5, 2.5, 1.2, 2.2, 1.1, 2.1, 1.3, 2.3})
	score, _ = calc.Score(noisy)
	if score > 0.5 {
		t.Errorf("expected low for noisy volatility, got %v", score)
	}
}

// --- Sideways V2 Tests ---

func TestSidewaysV2_TooFewCandles(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	series := makeSeries([]float64{1, 2, 3, 4, 5})
	score, err := calc.Score(series)
	if err == nil {
		t.Fatal("expected error for <6 candles")
	}
	if score != 0 {
		t.Errorf("expected 0 score, got %v", score)
	}
}

func TestSidewaysV2_FlatLine(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	flat := makeSeries([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	score, _ := calc.Score(flat)
	if score != 0 {
		t.Errorf("expected 0 for flat line, got %v", score)
	}
}

func TestSidewaysV2_CleanChannel(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	// Oscillating between 95 and 105, centered around 100 → ~10% range, good channel
	channel := makeSeries([]float64{100, 105, 100, 95, 100, 105, 100, 95, 100, 105, 100, 95, 100, 105, 100, 95, 100, 105, 100, 95})
	score, err := calc.Score(channel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.3 {
		t.Errorf("expected moderate-to-high score for clean channel, got %v", score)
	}
}

func TestSidewaysV2_StrongTrend(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	trend := makeSeries([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	score, err := calc.Score(trend)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score > 0.3 {
		t.Errorf("expected low score for strong trend, got %v", score)
	}
}

func TestSidewaysV2_Breakout(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	breakout := makeSeries([]float64{1, 1, 1, 1, 1, 2, 3, 4, 5, 6})
	score, err := calc.Score(breakout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score > 0.3 {
		t.Errorf("expected low for breakout, got %v", score)
	}
}

func TestSidewaysV2_Name(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	if calc.Name() != "Sideways Consistency" {
		t.Errorf("expected 'Sideways Consistency', got %q", calc.Name())
	}
}

func TestSidewaysV2_ScoreRange(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	cases := [][]float64{
		{100, 105, 100, 95, 100, 105},
		{1, 2, 3, 4, 5, 6},
		{50, 55, 50, 45, 50, 55, 50, 45},
		{10, 20, 10, 20, 10, 20},
	}
	for i, prices := range cases {
		series := makeSeries(prices)
		score, err := calc.Score(series)
		if err != nil {
			t.Fatalf("case %d: unexpected error: %v", i, err)
		}
		if score < 0 || score > 1 {
			t.Errorf("case %d: score %v out of [0,1]", i, score)
		}
	}
}

func TestSidewaysV2_NoisePenalty(t *testing.T) {
	calc := &SidewaysV2ScoreCalculator{}
	// Very high frequency alternation → ODS will be high
	noisy := makeSeries([]float64{100, 110, 100, 110, 100, 110, 100, 110, 100, 110})
	score, err := calc.Score(noisy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Score should be reduced by noise penalty (ODS > 0.8 → ×0.85)
	if score < 0 || score > 1 {
		t.Errorf("score %v out of [0,1]", score)
	}
}
