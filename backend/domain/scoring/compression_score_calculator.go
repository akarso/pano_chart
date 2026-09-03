package scoring

import (
	"math"
	"pano_chart/backend/domain"
)

// CompressionConfig holds all tunable parameters for the Compression Detector.
// No hardcoded magic numbers — every threshold is configurable.
type CompressionConfig struct {
	SwingLookback      int     // N for swing detection (local extrema window)
	ATRPeriod          int     // period for rolling ATR computation
	MinStructuralRange float64 // minimum price range (fraction) to consider structural
	WidthWeight        float64 // exponent for WCS in multiplicative core
	VolWeight          float64 // exponent for VCS in multiplicative core
	ConvergenceWeight  float64 // exponent for BCS in multiplicative core
	TouchFactor        float64 // ATR multiplier for boundary touch epsilon
	PressureThreshold  float64 // directional pressure threshold for bias classification
	SmallRangePenalty  float64 // penalty multiplier for tiny-range structures
	MaxExpectedSlope   float64 // normalization denominator for width slope
	MaxExpectedATRDrop float64 // normalization denominator for ATR slope
	SlopeNormalization float64 // normalization denominator for boundary convergence
	CandleCount        int     // number of candles to analyse (tail window)
}

// DefaultCompressionConfig returns the CompressionConfig from config.yaml
// (or hardcoded defaults when config.yaml is not loaded, e.g. in tests).
func DefaultCompressionConfig() CompressionConfig {
	return NewCompressionConfig()
}

// CompressionResult holds the output of the compression detector.
type CompressionResult struct {
	Score float64 // normalised [0,1]
	Bias  string  // "up", "down", "neutral"

	WidthContractionScore      float64
	VolatilityContractionScore float64
	BoundaryConvergenceScore   float64
	DirectionalPressureScore   float64

	ChannelWidthSlope float64
	ATRSlope          float64
}

// DetectCompression runs the geometry-agnostic structural tension engine.
//
// It produces a CompressionScore composed of four independent structural
// components: width contraction, volatility contraction, boundary convergence,
// and directional pressure.
func DetectCompression(candles []domain.Candle, cfg CompressionConfig) CompressionResult {
	if len(candles) < 10 {
		return CompressionResult{Bias: "neutral"}
	}

	// --- 1. Swing detection ---
	highsIdx, lowsIdx, _ := detectExtrema(candles, cfg.SwingLookback)
	if len(highsIdx) < 2 || len(lowsIdx) < 2 {
		return CompressionResult{Bias: "neutral"}
	}

	// --- 2. Channel construction: separate regressions through highs and lows ---
	upperSlope, upperIntercept := regressionThroughHighs(candles, highsIdx)
	lowerSlope, lowerIntercept := regressionThroughLows(candles, lowsIdx)

	// --- 3. Width series ---
	n := len(candles)
	widthSeries := make([]float64, n)
	for i := 0; i < n; i++ {
		upper := upperSlope*float64(i) + upperIntercept
		lower := lowerSlope*float64(i) + lowerIntercept
		w := upper - lower
		if w < 0 {
			w = 0
		}
		widthSeries[i] = w
	}

	// --- 4. Width Contraction Score (WCS) ---
	widthSlope, _ := linearRegressionFloats(widthSeries)
	WCS := computeContractionScore(widthSlope, cfg.MaxExpectedSlope)

	// --- 5. Volatility Contraction Score (VCS) ---
	atrSeries := rollingATR(candles, cfg.ATRPeriod)
	var atrSlope float64
	if len(atrSeries) >= 2 {
		atrSlope, _ = linearRegressionFloats(atrSeries)
	}
	VCS := computeContractionScore(atrSlope, cfg.MaxExpectedATRDrop)

	// --- 6. Boundary Convergence Score (BCS) ---
	BCS := computeBoundaryConvergence(upperSlope, lowerSlope, cfg.SlopeNormalization)

	// --- 7. Directional Pressure Score (DPS) + Bias ---
	atr := averageATR(candles)
	DPS, bias := computeDirectionalPressure(candles, upperSlope, upperIntercept, lowerSlope, lowerIntercept, atr, cfg)

	// --- 8. Composition: multiplicative core + pressure influence ---
	core := math.Pow(WCS, cfg.WidthWeight) *
		math.Pow(VCS, cfg.VolWeight) *
		math.Pow(BCS, cfg.ConvergenceWeight)

	score := core * (0.5 + 0.5*DPS)

	// --- 9. Edge case: tiny range penalty ---
	score = applySmallRangePenalty(candles, score, cfg)

	score = clamp(score, 0, 1)

	return CompressionResult{
		Score:                      score,
		Bias:                       bias,
		WidthContractionScore:      WCS,
		VolatilityContractionScore: VCS,
		BoundaryConvergenceScore:   BCS,
		DirectionalPressureScore:   DPS,
		ChannelWidthSlope:          widthSlope,
		ATRSlope:                   atrSlope,
	}
}

// computeContractionScore normalises a negative slope into [0,1].
// Non-negative slopes → 0 (no contraction).
func computeContractionScore(slope, maxExpected float64) float64 {
	if slope >= 0 || maxExpected <= 0 {
		return 0
	}
	return clamp(-slope/maxExpected, 0, 1)
}

// computeBoundaryConvergence scores how fast upper/lower boundaries converge.
func computeBoundaryConvergence(upperSlope, lowerSlope, slopeNorm float64) float64 {
	if slopeNorm <= 0 {
		return 0
	}
	slopeDelta := math.Abs(upperSlope - lowerSlope)
	bcs := clamp(slopeDelta/slopeNorm, 0, 1)
	// Bonus: true convergence (opposite-sign slopes) → boost
	if upperSlope < 0 && lowerSlope > 0 {
		bcs = clamp(bcs*1.2, 0, 1)
	}
	return bcs
}

// computeDirectionalPressure counts boundary touches and returns DPS + bias.
func computeDirectionalPressure(
	candles []domain.Candle,
	uSlope, uIntercept, lSlope, lIntercept, atr float64,
	cfg CompressionConfig,
) (float64, string) {
	epsilon := atr * cfg.TouchFactor
	var upperTouches, lowerTouches int
	for i, c := range candles {
		upper := uSlope*float64(i) + uIntercept
		lower := lSlope*float64(i) + lIntercept
		if c.High() >= upper-epsilon {
			upperTouches++
		}
		if c.Low() <= lower+epsilon {
			lowerTouches++
		}
	}
	totalTouches := upperTouches + lowerTouches
	if totalTouches == 0 {
		return 0, "neutral"
	}
	pressure := float64(upperTouches-lowerTouches) / float64(totalTouches)
	bias := "neutral"
	if pressure > cfg.PressureThreshold {
		bias = "up"
	} else if pressure < -cfg.PressureThreshold {
		bias = "down"
	}
	return math.Abs(pressure), bias
}

// applySmallRangePenalty reduces the score when the price range is too narrow
// to be structurally meaningful.
func applySmallRangePenalty(candles []domain.Candle, score float64, cfg CompressionConfig) float64 {
	maxHigh, minLow, meanPrice := extremesAndMean(candles)
	rangePercent := (maxHigh - minLow) / (meanPrice + 1e-6)
	if rangePercent < cfg.MinStructuralRange {
		score *= cfg.SmallRangePenalty
	}
	return score
}

// --- CompressionScoreCalculator implements SymbolScoreCalculator ---

// CompressionScoreCalculator adapts DetectCompression to the SymbolScoreCalculator interface.
type CompressionScoreCalculator struct {
	Config CompressionConfig
}

func (c *CompressionScoreCalculator) Name() string {
	return "Compression"
}

func (c *CompressionScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	count := c.Config.CandleCount
	length := series.Len()
	start := 0
	if length > count {
		start = length - count
		length = count
	}
	candles := make([]domain.Candle, length)
	for i := 0; i < length; i++ {
		cd, err := series.At(start + i)
		if err != nil {
			return 0, err
		}
		candles[i] = cd
	}
	res := DetectCompression(candles, c.Config)
	return res.Score, nil
}

// --- CompressionSubscore implements Subscore for MetaScore Engine ---

// CompressionSubscore wraps the compression detector for MetaScore composition.
type CompressionSubscore struct{}

func (s CompressionSubscore) Name() string {
	return "Compression"
}

func (s CompressionSubscore) Compute(data interface{}, cfg interface{}) SubscoreResult {
	candles, ok := data.([]domain.Candle)
	if !ok {
		return SubscoreResult{Value: 0, Confidence: 1.0, Meta: map[string]float64{"error": 1}}
	}
	compCfg, ok := cfg.(CompressionConfig)
	if !ok {
		compCfg = NewCompressionConfig()
	}
	res := DetectCompression(candles, compCfg)
	return SubscoreResult{
		Value:      clamp(res.Score, 0, 1),
		Confidence: 1.0,
		Meta: map[string]float64{
			"WCS":  res.WidthContractionScore,
			"VCS":  res.VolatilityContractionScore,
			"BCS":  res.BoundaryConvergenceScore,
			"DPS":  res.DirectionalPressureScore,
			"wSlp": res.ChannelWidthSlope,
			"aSlp": res.ATRSlope,
		},
	}
}

// --- Compression-specific helpers ---

// linearRegressionFloats computes slope and intercept for a float64 series
// indexed 0, 1, 2, …
func linearRegressionFloats(vals []float64) (slope, intercept float64) {
	n := float64(len(vals))
	if n < 2 {
		return 0, 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for i, v := range vals {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

// regressionThroughHighs fits y = slope*x + intercept through (index, High) pairs.
func regressionThroughHighs(candles []domain.Candle, indices []int) (slope, intercept float64) {
	if len(indices) < 2 {
		return 0, 0
	}
	n := float64(len(indices))
	var sumX, sumY, sumXY, sumXX float64
	for _, idx := range indices {
		x := float64(idx)
		y := candles[idx].High()
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

// regressionThroughLows fits y = slope*x + intercept through (index, Low) pairs.
func regressionThroughLows(candles []domain.Candle, indices []int) (slope, intercept float64) {
	if len(indices) < 2 {
		return 0, 0
	}
	n := float64(len(indices))
	var sumX, sumY, sumXY, sumXX float64
	for _, idx := range indices {
		x := float64(idx)
		y := candles[idx].Low()
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, sumY / n
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

// rollingATR computes a rolling ATR series with the given period.
func rollingATR(candles []domain.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return nil
	}
	// True range series (needs previous close, so starts at index 1)
	trs := make([]float64, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		r1 := candles[i].High() - candles[i].Low()
		r2 := math.Abs(candles[i].High() - candles[i-1].Close())
		r3 := math.Abs(candles[i].Low() - candles[i-1].Close())
		trs[i-1] = math.Max(r1, math.Max(r2, r3))
	}
	// Rolling average
	result := make([]float64, len(trs)-period+1)
	for i := 0; i <= len(trs)-period; i++ {
		sum := 0.0
		for j := 0; j < period; j++ {
			sum += trs[i+j]
		}
		result[i] = sum / float64(period)
	}
	return result
}

// LinearRegressionFloatsExported exposes linearRegressionFloats for testing.
func LinearRegressionFloatsExported(vals []float64) (float64, float64) {
	return linearRegressionFloats(vals)
}

// RollingATRExported exposes rollingATR for testing.
func RollingATRExported(candles []domain.Candle, period int) []float64 {
	return rollingATR(candles, period)
}
