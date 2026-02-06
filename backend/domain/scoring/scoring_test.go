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
