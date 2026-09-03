package scoring_test

import (
	"pano_chart/backend/domain/scoring"
	"testing"
)

// TestShapePenalty_CleanUptrend scores well — no penalty.
func TestShapePenalty_CleanUptrend(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// Clean linear uptrend: 100 → 150.
	prices := make([]float64, 100)
	for i := range prices {
		prices[i] = 100.0 + 50.0*float64(i)/99.0
	}
	score, err := calc.Score(makeCloseSeries(prices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.70 {
		t.Errorf("clean uptrend should score well, got %f", score)
	}
}

// TestShapePenalty_UptrendBrokenAtTail — collapses in last 15%.
func TestShapePenalty_UptrendBrokenAtTail(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// Uptrend 100→150 for first 85%, then collapse to 100 in last 15%.
	n := 100
	prices := make([]float64, n)
	pivot := n * 85 / 100
	for i := 0; i < pivot; i++ {
		prices[i] = 100.0 + 50.0*float64(i)/float64(pivot-1)
	}
	for i := pivot; i < n; i++ {
		frac := float64(i-pivot) / float64(n-1-pivot)
		prices[i] = 150.0 - 50.0*frac
	}
	score, err := calc.Score(makeCloseSeries(prices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The collapse in the tail should heavily penalize the score.
	if score > 0.30 {
		t.Errorf("broken uptrend should score low, got %f", score)
	}
}

// TestShapePenalty_UptrendFromDip — dip at start is tolerated.
func TestShapePenalty_UptrendFromDip(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// Dip in first 15%, then clean uptrend.
	n := 100
	prices := make([]float64, n)
	headEnd := n * 15 / 100
	// Dip from 130 down to 100 in head.
	for i := 0; i < headEnd; i++ {
		frac := float64(i) / float64(headEnd-1)
		prices[i] = 130.0 - 30.0*frac
	}
	// Then clean rise from 100 to 170.
	for i := headEnd; i < n; i++ {
		frac := float64(i-headEnd) / float64(n-1-headEnd)
		prices[i] = 100.0 + 70.0*frac
	}
	score, err := calc.Score(makeCloseSeries(prices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Head penalty is lenient — score should still be reasonable.
	if score < 0.20 {
		t.Errorf("uptrend from dip should be tolerated, got %f", score)
	}
}

// TestShapePenalty_DowntrendBrokenAtTail — rallies at end.
func TestShapePenalty_DowntrendBrokenAtTail(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// Downtrend 150→100 for first 85%, then rally to 150 in last 15%.
	n := 100
	prices := make([]float64, n)
	pivot := n * 85 / 100
	for i := 0; i < pivot; i++ {
		prices[i] = 150.0 - 50.0*float64(i)/float64(pivot-1)
	}
	for i := pivot; i < n; i++ {
		frac := float64(i-pivot) / float64(n-1-pivot)
		prices[i] = 100.0 + 50.0*frac
	}
	score, err := calc.Score(makeCloseSeries(prices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score > 0.30 {
		t.Errorf("broken downtrend should score low, got %f", score)
	}
}

// TestShapePenalty_ShortSeriesNoPenalty — < 10 candles → no penalty.
func TestShapePenalty_ShortSeriesNoPenalty(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	prices := []float64{100, 102, 104, 106, 108}
	score, err := calc.Score(makeCloseSeries(prices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score == 0 {
		t.Error("short uptrend should still score > 0")
	}
}
