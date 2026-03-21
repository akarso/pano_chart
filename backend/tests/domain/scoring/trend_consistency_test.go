package scoring_test

import (
	"math"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"testing"
	"time"
)

// makeCloseSeries creates a CandleSeries from close prices (open=close, small wicks).
func makeCloseSeries(prices []float64) domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := make([]domain.Candle, len(prices))
	for i, p := range prices {
		candles[i] = domain.NewCandleUnsafe(sym, tf,
			time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC),
			p, p+0.5, p-0.5, p, 1000)
	}
	series, _ := domain.NewCandleSeries(sym, tf, candles)
	return series
}

// TestTrend_LinearBeatsSteadily verifies that a clean linear trend
// scores the highest, beating both choppy and step-function patterns.
func TestTrend_LinearBeatsSteadily(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// Clean linear downtrend: 100 → 80
	linear := make([]float64, 150)
	for i := range linear {
		linear[i] = 100.0 - 20.0*float64(i)/149.0
	}
	linearScore, err := calc.Score(makeCloseSeries(linear))
	if err != nil {
		t.Fatalf("linear: %v", err)
	}

	// Choppy downtrend: same slope ± sine noise
	choppy := make([]float64, 150)
	for i := range choppy {
		choppy[i] = 100.0 - 20.0*float64(i)/149.0 + 3.0*math.Sin(float64(i)*0.5)
	}
	choppyScore, _ := calc.Score(makeCloseSeries(choppy))

	if linearScore <= choppyScore {
		t.Errorf("linear (%.6f) should beat choppy (%.6f)", linearScore, choppyScore)
	}
}

// TestTrend_StepFunctionPenalised verifies that a step function (-|_) —
// flat, sudden drop, flat — scores dramatically lower than a smooth trend
// with the same total price change.
func TestTrend_StepFunctionPenalised(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// -|_ pattern: flat at 100, drop to 80, flat at 80
	step := make([]float64, 150)
	for i := 0; i < 50; i++ {
		step[i] = 100.0
	}
	for i := 50; i < 55; i++ {
		frac := float64(i-50) / 5.0
		step[i] = 100.0 - 20.0*frac
	}
	for i := 55; i < 150; i++ {
		step[i] = 80.0
	}
	stepScore, _ := calc.Score(makeCloseSeries(step))

	// Clean linear downtrend for comparison
	linear := make([]float64, 150)
	for i := range linear {
		linear[i] = 100.0 - 20.0*float64(i)/149.0
	}
	linearScore, _ := calc.Score(makeCloseSeries(linear))

	// Step function should be less than 10% of linear
	if stepScore > linearScore*0.1 {
		t.Errorf("step (%.6f) should be <10%% of linear (%.6f)", stepScore, linearScore)
	}
}

// TestTrend_StaircaseScoredModerately verifies that a multi-step staircase
// (flat-step-flat-step-flat) is not zero — it has genuine trend character.
// The cluster gate catches pure 2-level step functions; a 3-step staircase
// bridges the gap.  After (N-1) normalization it scores near 1.0 because
// the overall slope and R² are high — this is valid directional behaviour.
func TestTrend_StaircaseScoredModerately(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	// 3-step staircase: 100 → jump → 110 → jump → 120
	stair := make([]float64, 150)
	for i := 0; i < 50; i++ {
		stair[i] = 100.0
	}
	for i := 50; i < 55; i++ {
		stair[i] = 100.0 + 10.0*float64(i-50)/5.0
	}
	for i := 55; i < 100; i++ {
		stair[i] = 110.0
	}
	for i := 100; i < 105; i++ {
		stair[i] = 110.0 + 10.0*float64(i-100)/5.0
	}
	for i := 105; i < 150; i++ {
		stair[i] = 120.0
	}
	stairScore, _ := calc.Score(makeCloseSeries(stair))

	// Staircase should score positively — it IS a directional move.
	if stairScore < 0.5 {
		t.Errorf("3-step staircase should score >= 0.5 (strong directionality), got %.6f", stairScore)
	}
}

// TestTrend_SymmetricUpDown verifies that the absolute score is the same
// for an uptrend and a downtrend of equal magnitude.
func TestTrend_SymmetricUpDown(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	up := make([]float64, 150)
	down := make([]float64, 150)
	for i := range up {
		up[i] = 80.0 + 40.0*float64(i)/149.0
		down[i] = 120.0 - 40.0*float64(i)/149.0
	}
	upScore, _ := calc.Score(makeCloseSeries(up))
	downScore, _ := calc.Score(makeCloseSeries(down))

	diff := math.Abs(upScore - downScore)
	if diff > 1e-10 {
		t.Errorf("up (%.6f) and down (%.6f) should be symmetric", upScore, downScore)
	}
}

// TestTrend_FlatLineReturnsZero: no movement → zero score.
func TestTrend_FlatLineReturnsZero(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	flat := make([]float64, 150)
	for i := range flat {
		flat[i] = 100.0
	}
	score, _ := calc.Score(makeCloseSeries(flat))
	if score != 0 {
		t.Errorf("flat line should score 0, got %.6f", score)
	}
}
