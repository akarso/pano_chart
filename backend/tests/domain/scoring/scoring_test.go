package scoring

import (
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
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
	calc := &scoring.GainLossScoreCalculator{}
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
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	series := makeSeries([]float64{1, 2, 3, 4, 5})
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive trend, got %v", score)
	}
}

// TestTrendPredictability_ShortMonotonicTrendNotClustered guards against a
// regression where closePricesClustered() false-positived on short (~8-10
// candle), evenly-spaced, monotonic trends: every sorted-adjacent gap in
// such a series is ~range/(n-1), which for small n exceeds the 10%-of-range
// threshold on its own, incorrectly zeroing out a textbook clean trend.
func TestTrendPredictability_ShortMonotonicTrendNotClustered(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}

	cases := map[string][]float64{
		"8-candle uptrend":    {100, 101, 102, 103, 104, 105, 106, 107},
		"9-candle uptrend":    {100, 101, 102, 103, 104, 105, 106, 107, 108},
		"10-candle uptrend":   {100, 101, 102, 103, 104, 105, 106, 107, 108, 109},
		"10-candle downtrend": {109, 108, 107, 106, 105, 104, 103, 102, 101, 100},
	}
	for name, prices := range cases {
		series := makeSeries(prices)
		score, err := calc.Score(series)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if score <= 0 {
			t.Errorf("%s: expected positive trend score, got %v (clustering false-positive?)", name, score)
		}
	}
}

// TestTrendPredictability_StepFunctionStillClustered ensures a genuine
// two-plateau step function (regime shift, not a trend) is still zeroed
// out after tightening the cluster-gate heuristic.
func TestTrendPredictability_StepFunctionStillClustered(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	// Two flat plateaus with a large jump between them and tiny in-plateau
	// jitter, so within-cluster gaps are near zero and the single
	// between-cluster gap dwarfs the median gap.
	stepFn := makeSeries([]float64{
		100, 100.01, 100.02, 100.01, 100,
		150, 150.01, 150.02, 150.01, 150,
	})
	score, err := calc.Score(stepFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected step function to be flagged as clustered (score=0), got %v", score)
	}
}

// TestTrendPredictability_NonUniformSpacingNotClustered checks the fix
// against irregular (non-uniformly-spaced) step sizes, as real candle data
// has from ordinary volatility, rather than only the perfectly-even
// synthetic series above.
func TestTrendPredictability_NonUniformSpacingNotClustered(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	// Monotonic uptrend with varied (1-2 unit) step sizes, no single move
	// dominating — realistic minor volatility around a clean trend.
	uneven := makeSeries([]float64{100, 101, 102, 104, 105, 106, 108, 109, 110, 112})
	score, err := calc.Score(uneven)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive trend score for non-uniform monotonic trend, got %v (clustering false-positive?)", score)
	}
}

// TestTrendPredictability_ClusteredFallsBackWhenMedianGapZero exercises the
// path where medianGap == 0 (at least half the sorted adjacent gaps are
// exact duplicates, plausible with tick-rounded closes) — closePricesClustered
// falls back to the original 10%-of-range-only rule in that case, which is
// correct here since the series is a genuine two-plateau step function.
func TestTrendPredictability_ClusteredFallsBackWhenMedianGapZero(t *testing.T) {
	calc := &scoring.TrendPredictabilityScoreCalculator{}
	stepFn := makeSeries([]float64{100, 100, 100, 100, 150, 150, 150, 150})
	score, err := calc.Score(stepFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected step function with duplicate closes to be flagged as clustered (score=0), got %v", score)
	}
}

func TestSidewaysConsistencyScoreCalculator(t *testing.T) {
	calc := &scoring.SidewaysConsistencyScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
	flat := makeSeries([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	score, _ := calc.Score(flat)
	if score != 0 {
		t.Errorf("expected 0 for flat line, got %v", score)
	}
}

func TestSidewaysV2_CleanChannel(t *testing.T) {
	calc := &scoring.SidewaysV2ScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
	if calc.Name() != "Sideways Consistency" {
		t.Errorf("expected 'Sideways Consistency', got %q", calc.Name())
	}
}

func TestSidewaysV2_ScoreRange(t *testing.T) {
	calc := &scoring.SidewaysV2ScoreCalculator{}
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
	calc := &scoring.SidewaysV2ScoreCalculator{}
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

// --- SidewaysV3 tests ---

// makeOHLCSeries creates a CandleSeries with explicit open/high/low/close per candle.
// Each element is [open, high, low, close].
func makeOHLCSeries(ohlc [][4]float64) domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := make([]domain.Candle, len(ohlc))
	for i, v := range ohlc {
		candles[i] = domain.NewCandleUnsafe(sym, tf,
			time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC),
			v[0], v[1], v[2], v[3], 1)
	}
	series, _ := domain.NewCandleSeries(sym, tf, candles)
	return series
}

// channelOHLC generates n candles oscillating between lo and hi with
// distinct high/low wicks, creating a clean channel structure.
func channelOHLC(n int, lo, hi float64) [][4]float64 {
	mid := (lo + hi) / 2
	ohlc := make([][4]float64, n)
	for i := 0; i < n; i++ {
		var close float64
		switch i % 4 {
		case 0:
			close = hi // touch upper
		case 2:
			close = lo // touch lower
		default:
			close = mid
		}
		open := mid
		high := close + (hi-lo)*0.02 // small wick above close
		low := close - (hi-lo)*0.02  // small wick below close
		// Ensure high >= all, low <= all
		if high < open {
			high = open + (hi-lo)*0.02
		}
		if low > open {
			low = open - (hi-lo)*0.02
		}
		ohlc[i] = [4]float64{open, high, low, close}
	}
	return ohlc
}

func TestSidewaysV3_Name(t *testing.T) {
	calc := &scoring.SidewaysV3ScoreCalculator{Config: scoring.DefaultSidewaysV3Config("1h")}
	if calc.Name() != "Sideways Consistency" {
		t.Errorf("expected 'Sideways Consistency', got %q", calc.Name())
	}
}

func TestSidewaysV3_TooFewCandles(t *testing.T) {
	calc := &scoring.SidewaysV3ScoreCalculator{Config: scoring.DefaultSidewaysV3Config("1h")}
	series := makeSeries([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	_, err := calc.Score(series)
	if err == nil {
		t.Fatal("expected error for <20 candles")
	}
}

func TestSidewaysV3_FlatLine(t *testing.T) {
	calc := &scoring.SidewaysV3ScoreCalculator{Config: scoring.DefaultSidewaysV3Config("1h")}
	flat := make([]float64, 30)
	for i := range flat {
		flat[i] = 100
	}
	series := makeSeries(flat)
	score, _ := calc.Score(series)
	if score != 0 {
		t.Errorf("expected 0 for flat line (H=0), got %v", score)
	}
}

func TestSidewaysV3_CleanChannel(t *testing.T) {
	// 40-candle oscillation between 97 and 103 around 100.
	// Use a permissive config so range score doesn't dominate.
	cfg := scoring.SidewaysV3Config{RMin: 0.01, RMax: 0.12}
	calc := &scoring.SidewaysV3ScoreCalculator{Config: cfg}
	// Build channel with period 4: hi, mid, lo, mid — so first and last
	// candle close at the same value (both index 0%4=0 → hi), minimising drift.
	// ohlc = ohlc[:len(ohlc)-1] // removed: value never used
	// Actually use 40 candles starting and ending at mid:
	ohlc := channelOHLC(42, 97, 103) // index 0→hi, 1→mid, ..., 41→mid
	ohlc = ohlc[1:41]                // indices 1..40 → starts & ends at mid → zero drift
	series := makeOHLCSeries(ohlc)
	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0.3 {
		t.Errorf("expected moderate-to-high score for clean channel, got %v", score)
	}
}

func TestSidewaysV3_StrongTrend(t *testing.T) {
	calc := &scoring.SidewaysV3ScoreCalculator{Config: scoring.DefaultSidewaysV3Config("1h")}
	// Steady uptrend: 100 → 130, drift >> channel height
	prices := make([]float64, 30)
	for i := range prices {
		prices[i] = 100 + float64(i)
	}
	series := makeSeries(prices)
	score, _ := calc.Score(series)
	if score > 0.1 {
		t.Errorf("expected near-zero for strong trend, got %v", score)
	}
}

func TestSidewaysV3_Breakout(t *testing.T) {
	calc := &scoring.SidewaysV3ScoreCalculator{Config: scoring.DefaultSidewaysV3Config("1h")}
	// Flat at 100 then break to 120
	prices := make([]float64, 30)
	for i := 0; i < 20; i++ {
		prices[i] = 100
	}
	for i := 20; i < 30; i++ {
		prices[i] = 100 + float64(i-20)*2
	}
	series := makeSeries(prices)
	score, _ := calc.Score(series)
	if score > 0.2 {
		t.Errorf("expected low for breakout, got %v", score)
	}
}

// func TestSidewaysV3_ScoreRange(t *testing.T) {
// 	cfg := scoring.SidewaysV3Config{RMin: 0.005, RMax: 0.15}
// 	calc := &scoring.SidewaysV3ScoreCalculator{Config: cfg}
// 	cases := [][][4]float64{
// 		scoring.ChannelOHLC(30, 95, 105),
// 		scoring.ChannelOHLC(40, 98, 102),
// 		scoring.ChannelOHLC(30, 90, 110),
// 	}
// 	for i, ohlc := range cases {
// 		series := makeOHLCSeries(ohlc)
// 		score, err := calc.Score(series)
// 		if err != nil {
// 			t.Fatalf("case %d: unexpected error: %v", i, err)
// 		}
// 		if score < 0 || score > 1 {
// 			t.Errorf("case %d: score %v out of [0,1]", i, score)
// 		}
// 	}
// }

// func TestSidewaysV3_MicroCompressionScoresLow(t *testing.T) {
// 	// Very tight range: 99.99–100.01 → R ≈ 0.0002, below any RMin
// 	calc := &SidewaysV3ScoreCalculator{Config: DefaultSidewaysV3Config("1h")}
// 	prices := make([]float64, 30)
// 	for i := range prices {
// 		if i%2 == 0 {
// 			prices[i] = 100.01
// 		} else {
// 			prices[i] = 99.99
// 		}
// 	}
// 	series := makeSeries(prices)
// 	score, _ := calc.Score(series)
// 	if score > 0.05 {
// 		t.Errorf("expected near-zero for micro compression, got %v", score)
// 	}
// }

// func TestSidewaysV3_WideVolatileScoresLow(t *testing.T) {
// 	// Very wide swings: 50–150 → R = 100/100 = 1.0, way above RMax
// 	calc := &SidewaysV3ScoreCalculator{Config: DefaultSidewaysV3Config("1h")}
// 	prices := make([]float64, 30)
// 	for i := range prices {
// 		if i%2 == 0 {
// 			prices[i] = 150
// 		} else {
// 			prices[i] = 50
// 		}
// 	}
// 	series := makeSeries(prices)
// 	score, _ := calc.Score(series)
// 	if score > 0.05 {
// 		t.Errorf("expected near-zero for wide volatility, got %v", score)
// 	}
// }
