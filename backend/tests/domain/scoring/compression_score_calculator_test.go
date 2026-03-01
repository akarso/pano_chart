package scoring

import (
	"math"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"testing"
	"time"
)

// makeCompressionCandles creates candles with explicit OHLCV data.
// Each entry: [open, high, low, close, volume].
func makeCompressionCandles(data [][5]float64) []domain.Candle {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := make([]domain.Candle, len(data))
	for i, d := range data {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf,
			time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC),
			d[0], d[1], d[2], d[3], d[4],
		)
	}
	return candles
}

// makeCompressionSeries wraps makeCompressionCandles into a CandleSeries.
func makeCompressionSeries(data [][5]float64) domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("TEST")
	tf := domain.NewTimeframeUnsafe("1h")
	candles := makeCompressionCandles(data)
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

// convergingChannel generates candles with clear swing highs/lows whose envelope
// converges over time (highs decrease, lows increase). Every 7 candles a swing
// high reaches the upper boundary; offset by 3, a swing low reaches the lower.
// This creates detectable extrema for the N=3 swing detector.
func convergingChannel(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		// Converging envelope
		upper := 110.0 - float64(i)*0.15
		lower := 90.0 + float64(i)*0.15
		mid := (upper + lower) / 2

		// Default: moderate candle near mid
		h := mid + 2
		l := mid - 2
		o := mid - 0.5
		c := mid + 0.5

		// Every 7 bars: swing high (reach upper boundary)
		if i%7 == 0 {
			h = upper
			c = upper - 1
			o = mid
		}
		// Offset by 3: swing low (reach lower boundary)
		if i%7 == 3 {
			l = lower
			c = lower + 1
			o = mid
		}

		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}
	return data
}

// expandingChannel generates candles where highs increase and lows decrease.
func expandingChannel(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		upper := 102.0 + float64(i)*0.3
		lower := 98.0 - float64(i)*0.3
		mid := (upper + lower) / 2

		h := mid + 1
		l := mid - 1
		o := mid - 0.3
		c := mid + 0.3

		if i%7 == 0 {
			h = upper
			c = upper - 0.5
			o = mid
		}
		if i%7 == 3 {
			l = lower
			c = lower + 0.5
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}
	return data
}

// flatChannel generates a channel with constant width and clear swing points.
func flatChannel(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		mid := 100.0
		h := mid + 2
		l := mid - 2
		o := mid - 0.5
		c := mid + 0.5

		if i%7 == 0 {
			h = 105.0
			c = 104.0
			o = mid
		}
		if i%7 == 3 {
			l = 95.0
			c = 96.0
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}
	return data
}

// tinyRangeChannel generates candles with very small price variation.
func tinyRangeChannel(n int) [][5]float64 {
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		mid := 100.0
		h := mid + 0.001 - float64(i)*0.00001
		l := mid - 0.001 + float64(i)*0.00001
		var o, c float64
		if i%2 == 0 {
			o = mid - 0.0005
			c = mid + 0.0005
		} else {
			o = mid + 0.0005
			c = mid - 0.0005
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}
	return data
}

// --- DetectCompression tests ---

func TestDetectCompression_TooFewCandles(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(convergingChannel(5))
	res := scoring.DetectCompression(candles, cfg)

	if res.Score != 0 {
		t.Errorf("expected score 0 for too few candles, got %v", res.Score)
	}
	if res.Bias != "neutral" {
		t.Errorf("expected neutral bias, got %v", res.Bias)
	}
}

func TestDetectCompression_ConvergingChannel_PositiveScore(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(convergingChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	if res.Score <= 0 {
		t.Errorf("expected positive compression score for converging channel, got %v", res.Score)
	}
	if res.Score > 1 {
		t.Errorf("score should be <= 1, got %v", res.Score)
	}
	if res.WidthContractionScore <= 0 {
		t.Errorf("expected positive WCS for converging channel, got %v", res.WidthContractionScore)
	}
}

func TestDetectCompression_ExpandingChannel_LowScore(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(expandingChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	if res.WidthContractionScore != 0 {
		t.Errorf("expected WCS = 0 for expanding channel, got %v", res.WidthContractionScore)
	}
	if res.Score > 0.01 {
		t.Errorf("expected near-zero score for expanding channel, got %v", res.Score)
	}
}

func TestDetectCompression_FlatChannel_LowScore(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(flatChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	if res.Score > 0.1 {
		t.Errorf("expected low score for flat channel, got %v", res.Score)
	}
}

func TestDetectCompression_AllSubscoresNormalized(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(convergingChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	checks := map[string]float64{
		"Score": res.Score,
		"WCS":   res.WidthContractionScore,
		"VCS":   res.VolatilityContractionScore,
		"BCS":   res.BoundaryConvergenceScore,
		"DPS":   res.DirectionalPressureScore,
	}
	for name, v := range checks {
		if v < 0 || v > 1 {
			t.Errorf("%s outside [0,1]: %v", name, v)
		}
	}
}

func TestDetectCompression_BiasValues(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	candles := makeCompressionCandles(convergingChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	validBias := map[string]bool{"up": true, "down": true, "neutral": true}
	if !validBias[res.Bias] {
		t.Errorf("unexpected bias value: %q", res.Bias)
	}
}

func TestDetectCompression_TinyRangePenalty(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	cfg.MinStructuralRange = 0.01
	candles := makeCompressionCandles(tinyRangeChannel(60))
	res := scoring.DetectCompression(candles, cfg)

	if res.Score > 0.3 {
		t.Errorf("expected penalised score for tiny range, got %v", res.Score)
	}
}

func TestDetectCompression_DirectionalPressure_UpBias(t *testing.T) {
	// Build converging channel where closes bias toward upper boundary
	n := 60
	data := make([][5]float64, n)
	for i := 0; i < n; i++ {
		upper := 110.0 - float64(i)*0.15
		lower := 90.0 + float64(i)*0.15
		mid := (upper + lower) / 2

		// Default: candle near upper half → more upper touches
		h := mid + 2
		l := mid - 1
		o := mid + 1
		c := upper - 2 // close near upper

		if i%7 == 0 {
			h = upper
			c = upper - 0.5
			o = mid
		}
		if i%7 == 3 {
			l = lower
			c = lower + 3 // still bias up
			o = mid
		}
		if h < math.Max(o, c) {
			h = math.Max(o, c)
		}
		if l > math.Min(o, c) {
			l = math.Min(o, c)
		}
		data[i] = [5]float64{o, h, l, c, 100}
	}

	cfg := scoring.DefaultCompressionConfig()
	cfg.PressureThreshold = 0.1
	candles := makeCompressionCandles(data)
	res := scoring.DetectCompression(candles, cfg)

	if res.DirectionalPressureScore == 0 {
		t.Logf("result: %+v", res)
		t.Errorf("expected non-zero directional pressure")
	}
}

// --- CompressionScoreCalculator tests ---

func TestCompressionScoreCalculator_Name(t *testing.T) {
	calc := &scoring.CompressionScoreCalculator{Config: scoring.DefaultCompressionConfig()}
	if calc.Name() != "Compression" {
		t.Errorf("expected name 'Compression', got %q", calc.Name())
	}
}

func TestCompressionScoreCalculator_ScoreInRange(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	calc := &scoring.CompressionScoreCalculator{Config: cfg}
	series := makeCompressionSeries(convergingChannel(60))

	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0 || score > 1 {
		t.Errorf("score outside [0,1]: %v", score)
	}
}

func TestCompressionScoreCalculator_TailWindowTrimming(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	cfg.CandleCount = 30
	calc := &scoring.CompressionScoreCalculator{Config: cfg}
	series := makeCompressionSeries(convergingChannel(60))

	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < 0 || score > 1 {
		t.Errorf("score outside [0,1]: %v", score)
	}
}

func TestCompressionScoreCalculator_FewCandles(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()
	calc := &scoring.CompressionScoreCalculator{Config: cfg}
	series := makeCompressionSeries(convergingChannel(5))

	score, err := calc.Score(series)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected 0 for insufficient candles, got %v", score)
	}
}

// --- CompressionSubscore tests ---

func TestCompressionSubscore_Name(t *testing.T) {
	sub := scoring.CompressionSubscore{}
	if sub.Name() != "Compression" {
		t.Errorf("expected 'Compression', got %q", sub.Name())
	}
}

func TestCompressionSubscore_Compute(t *testing.T) {
	sub := scoring.CompressionSubscore{}
	candles := makeCompressionCandles(convergingChannel(60))
	res := sub.Compute(candles, scoring.DefaultCompressionConfig())

	if res.Value < 0 || res.Value > 1 {
		t.Errorf("value outside [0,1]: %v", res.Value)
	}
	if res.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %v", res.Confidence)
	}
	if _, ok := res.Meta["WCS"]; !ok {
		t.Error("expected WCS in meta")
	}
}

func TestCompressionSubscore_InvalidData(t *testing.T) {
	sub := scoring.CompressionSubscore{}
	res := sub.Compute("not candles", nil)
	if res.Value != 0 {
		t.Errorf("expected 0 for invalid data, got %v", res.Value)
	}
}

// --- Helper function tests ---

func TestLinearRegressionFloats_Ascending(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	slope, _ := scoring.LinearRegressionFloatsExported(vals)
	if math.Abs(slope-1.0) > 0.001 {
		t.Errorf("expected slope ~1.0, got %v", slope)
	}
}

func TestLinearRegressionFloats_Descending(t *testing.T) {
	vals := []float64{5, 4, 3, 2, 1}
	slope, _ := scoring.LinearRegressionFloatsExported(vals)
	if math.Abs(slope-(-1.0)) > 0.001 {
		t.Errorf("expected slope ~-1.0, got %v", slope)
	}
}

func TestLinearRegressionFloats_Constant(t *testing.T) {
	vals := []float64{3, 3, 3}
	slope, _ := scoring.LinearRegressionFloatsExported(vals)
	if math.Abs(slope) > 0.001 {
		t.Errorf("expected slope ~0, got %v", slope)
	}
}

func TestLinearRegressionFloats_TooFew(t *testing.T) {
	vals := []float64{10}
	slope, intercept := scoring.LinearRegressionFloatsExported(vals)
	if slope != 0 || intercept != 0 {
		t.Errorf("expected (0,0) for single element, got (%v,%v)", slope, intercept)
	}
}

func TestRollingATR_Basic(t *testing.T) {
	candles := makeCompressionCandles(convergingChannel(20))
	result := scoring.RollingATRExported(candles, 5)
	if result == nil {
		t.Fatal("expected non-nil rolling ATR")
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty rolling ATR")
	}
	for _, v := range result {
		if v < 0 {
			t.Errorf("ATR value should be non-negative, got %v", v)
		}
	}
}

func TestRollingATR_TooFewCandles(t *testing.T) {
	candles := makeCompressionCandles(convergingChannel(5))
	result := scoring.RollingATRExported(candles, 14)
	if result != nil {
		t.Errorf("expected nil for too few candles, got %v", result)
	}
}

// --- DefaultCompressionConfig tests ---

func TestDefaultCompressionConfig_SaneDefaults(t *testing.T) {
	cfg := scoring.DefaultCompressionConfig()

	if cfg.SwingLookback <= 0 {
		t.Error("SwingLookback must be positive")
	}
	if cfg.ATRPeriod <= 0 {
		t.Error("ATRPeriod must be positive")
	}
	if cfg.CandleCount <= 0 {
		t.Error("CandleCount must be positive")
	}
	if cfg.WidthWeight < 0 || cfg.VolWeight < 0 || cfg.ConvergenceWeight < 0 {
		t.Error("weights must be non-negative")
	}
	if cfg.PressureThreshold < 0 || cfg.PressureThreshold > 1 {
		t.Error("PressureThreshold must be in [0,1]")
	}
}
