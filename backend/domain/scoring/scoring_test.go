package scoring

import (
	"../"
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
	// Perfect oscillation
	series := makeSeries([]float64{1, 2, 1, 2, 1, 2, 1, 2, 1, 1})
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.7 {
		t.Errorf("expected high sideways score, got %v", score)
	}
	// Flat
	flat := makeSeries([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	score, _ = calc.Score(flat)
	if score != 0 {
		t.Errorf("expected zero for flat, got %v", score)
	}
	// Trend
	trend := makeSeries([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	score, _ = calc.Score(trend)
	if score > 0.2 {
		t.Errorf("expected low for trend, got %v", score)
	}
}
