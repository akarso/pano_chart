package scoring

import (
	"math"
	"pano_chart/backend/domain"
)

// BreakoutConfig holds all tunable parameters for the Breakout Detector.
type BreakoutConfig struct {
	ATRPeriod              int     // period for rolling ATR computation
	VolumeLookback         int     // lookback window for volume mean/std
	PenetrationNorm        float64 // denominator for boundary penetration normalisation
	ATRNorm                float64 // denominator for ATR slope normalisation
	VolumeNorm             float64 // denominator for volume z-score normalisation
	ReentryLookback        int     // candles to look back for re-entry penalty
	FailurePenalty         float64 // multiplier applied on false breakout re-entry
	CompressionThreshold   float64 // CompressionScore above this triggers boost
	CompressionBoostFactor float64 // multiplier for compression-based boost (e.g. 1.5)
	MinBodyRatio           float64 // minimum body-to-range ratio for close conviction
	SwingLookback          int     // N for swing detection (channel construction)
	CandleCount            int     // tail window of candles to analyse
	W1                     float64 // exponent for BVS in multiplicative core
	W2                     float64 // exponent for CCS in multiplicative core
	W3                     float64 // exponent for VES in multiplicative core
}

// DefaultBreakoutConfig returns the BreakoutConfig from config.yaml
// (or hardcoded defaults when config.yaml is not loaded, e.g. in tests).
func DefaultBreakoutConfig() BreakoutConfig {
	return NewBreakoutConfig()
}

// BreakoutResult holds the output of the breakout detector.
type BreakoutResult struct {
	UpScore   float64 // normalised [0,1]
	DownScore float64 // normalised [0,1]

	BoundaryViolationUp   float64
	BoundaryViolationDown float64
	CloseConvictionUp     float64
	CloseConvictionDown   float64
	VolatilityExpansion   float64
	VolumeScore           float64

	CompressionBoost float64

	UpperBoundary float64 // channel upper at last candle
	LowerBoundary float64 // channel lower at last candle
}

// DetectBreakout runs the structural release detector.
//
// It evaluates boundary violation, close conviction, volatility expansion,
// volume expansion, and optional compression boost to produce directional
// breakout scores.
func DetectBreakout(candles []domain.Candle, cfg BreakoutConfig, compressionScore float64) BreakoutResult {
	n := len(candles)
	if n < 15 {
		return BreakoutResult{CompressionBoost: 1.0}
	}

	// --- 1. Channel construction via swing extrema + regression ---
	highsIdx, lowsIdx, _ := detectExtrema(candles, cfg.SwingLookback)
	if len(highsIdx) < 2 || len(lowsIdx) < 2 {
		return BreakoutResult{CompressionBoost: 1.0}
	}

	upperSlope, upperIntercept := regressionThroughHighs(candles, highsIdx)
	lowerSlope, lowerIntercept := regressionThroughLows(candles, lowsIdx)

	lastIdx := float64(n - 1)
	upper := upperSlope*lastIdx + upperIntercept
	lower := lowerSlope*lastIdx + lowerIntercept

	// --- 2. ATR at last candle ---
	atrSeries := rollingATR(candles, cfg.ATRPeriod)
	if len(atrSeries) == 0 {
		return BreakoutResult{CompressionBoost: 1.0, UpperBoundary: upper, LowerBoundary: lower}
	}
	currentATR := atrSeries[len(atrSeries)-1]
	if currentATR <= 0 {
		return BreakoutResult{CompressionBoost: 1.0, UpperBoundary: upper, LowerBoundary: lower}
	}

	last := candles[n-1]

	// --- 3. Boundary Violation Score (BVS) ---
	bvsUp, bvsDown := computeBVS(last, upper, lower, currentATR, cfg.PenetrationNorm)

	// --- 4. Close Conviction Score (CCS) ---
	ccsUp, ccsDown := computeCCS(last, upper, lower, cfg.MinBodyRatio)

	// --- 5. Volatility Expansion Score (VES) ---
	ves := computeVES(atrSeries, cfg.ATRNorm)

	// --- 6. Volume Expansion Score (VLS) ---
	vls := computeVLS(candles, cfg.VolumeLookback, cfg.VolumeNorm)

	// --- 7. Compression Boost ---
	// When compression is above threshold, boost the breakout score.
	// High compression → stronger structural release → boost > 1.0.
	compBoost := 1.0
	if compressionScore > cfg.CompressionThreshold {
		boostFactor := cfg.CompressionBoostFactor
		if boostFactor <= 0 {
			boostFactor = 1.5 // safe default
		}
		compBoost = 1.0 + (boostFactor-1.0)*compressionScore
	}

	// --- 8. Composite scores ---
	// BVS and CCS form the multiplicative core (hard requirements).
	// VES and VLS are conviction modifiers: (0.5 + 0.5 * score) so they
	// boost but never zero-gate the core.
	coreUp := math.Pow(bvsUp, cfg.W1) *
		math.Pow(ccsUp, cfg.W2)
	coreUp *= (0.5 + 0.5*ves)
	coreUp *= (0.5 + 0.5*vls)
	upScore := coreUp * compBoost

	coreDown := math.Pow(bvsDown, cfg.W1) *
		math.Pow(ccsDown, cfg.W2)
	coreDown *= (0.5 + 0.5*ves)
	coreDown *= (0.5 + 0.5*vls)
	downScore := coreDown * compBoost

	// --- 9. Re-entry penalty (false breakout mitigation) ---
	upScore = applyReentryPenalty(candles, upScore, upper, "up", cfg)
	downScore = applyReentryPenalty(candles, downScore, lower, "down", cfg)

	upScore = clamp(upScore, 0, 1)
	downScore = clamp(downScore, 0, 1)

	return BreakoutResult{
		UpScore:               upScore,
		DownScore:             downScore,
		BoundaryViolationUp:   bvsUp,
		BoundaryViolationDown: bvsDown,
		CloseConvictionUp:     ccsUp,
		CloseConvictionDown:   ccsDown,
		VolatilityExpansion:   ves,
		VolumeScore:           vls,
		CompressionBoost:      compBoost,
		UpperBoundary:         upper,
		LowerBoundary:         lower,
	}
}

// computeBVS computes the Boundary Violation Score for up and down.
func computeBVS(c domain.Candle, upper, lower, atr, norm float64) (bvsUp, bvsDown float64) {
	if norm <= 0 {
		return 0, 0
	}
	penUp := (c.Close() - upper) / atr
	if penUp > 0 {
		bvsUp = clamp(penUp/norm, 0, 1)
	}
	penDown := (lower - c.Close()) / atr
	if penDown > 0 {
		bvsDown = clamp(penDown/norm, 0, 1)
	}
	return
}

// computeCCS computes the Close Conviction Score for up and down.
// bodyRatio must exceed minBodyRatio for the score to be non-zero.
func computeCCS(c domain.Candle, upper, lower float64, minBodyRatio float64) (ccsUp, ccsDown float64) {
	candleRange := c.High() - c.Low()
	if candleRange <= 0 {
		return 0, 0
	}
	bodySize := math.Abs(c.Close() - c.Open())
	bodyRatio := bodySize / candleRange

	if bodyRatio < minBodyRatio {
		return 0, 0
	}

	// Up conviction: close above upper AND bullish candle
	if c.Close() > upper && c.Close() > c.Open() {
		ccsUp = bodyRatio
	}
	// Down conviction: close below lower AND bearish candle
	if c.Close() < lower && c.Close() < c.Open() {
		ccsDown = bodyRatio
	}
	return
}

// computeVES computes the Volatility Expansion Score from ATR slope.
func computeVES(atrSeries []float64, atrNorm float64) float64 {
	if len(atrSeries) < 2 || atrNorm <= 0 {
		return 0
	}
	slope, _ := linearRegressionFloats(atrSeries)
	if slope <= 0 {
		return 0
	}
	return clamp(slope/atrNorm, 0, 1)
}

// computeVLS computes the Volume Expansion Score.
func computeVLS(candles []domain.Candle, lookback int, volumeNorm float64) float64 {
	n := len(candles)
	if n < 2 || lookback < 2 || volumeNorm <= 0 {
		return 0
	}

	// Determine lookback window (exclude last candle from baseline)
	start := n - 1 - lookback
	if start < 0 {
		start = 0
	}
	end := n - 1 // exclusive of last candle

	window := candles[start:end]
	if len(window) < 2 {
		return 0
	}

	// Mean and stddev of volume in window
	var sum float64
	for _, c := range window {
		sum += c.Volume()
	}
	mean := sum / float64(len(window))

	var sumSq float64
	for _, c := range window {
		d := c.Volume() - mean
		sumSq += d * d
	}
	std := math.Sqrt(sumSq / float64(len(window)))
	if std <= 0 {
		return 0
	}

	currentVol := candles[n-1].Volume()
	z := (currentVol - mean) / std
	if z <= 0 {
		return 0
	}
	return clamp(z/volumeNorm, 0, 1)
}

// applyReentryPenalty checks if price has come back inside the boundary
// within the lookback window, indicating a false breakout.
func applyReentryPenalty(candles []domain.Candle, score, boundary float64, direction string, cfg BreakoutConfig) float64 {
	n := len(candles)
	if score == 0 || cfg.ReentryLookback <= 0 {
		return score
	}

	start := n - 1 - cfg.ReentryLookback
	if start < 0 {
		start = 0
	}

	// Check candles before the last one for a violation followed by re-entry
	for i := start; i < n-1; i++ {
		c := candles[i]
		switch direction {
		case "up":
			// If a prior candle had closed above boundary but subsequent closed below
			if c.Close() > boundary {
				for j := i + 1; j < n; j++ {
					if candles[j].Close() < boundary {
						return score * cfg.FailurePenalty
					}
				}
			}
		case "down":
			// If a prior candle had closed below boundary but subsequent closed above
			if c.Close() < boundary {
				for j := i + 1; j < n; j++ {
					if candles[j].Close() > boundary {
						return score * cfg.FailurePenalty
					}
				}
			}
		}
	}
	return score
}

// --- BreakoutScoreCalculator implements SymbolScoreCalculator ---

// BreakoutScoreCalculator adapts DetectBreakout to the SymbolScoreCalculator interface.
// It produces max(UpScore, DownScore) as the single score.
type BreakoutScoreCalculator struct {
	Config           BreakoutConfig
	CompressionScore float64 // injected externally if available
}

func (b *BreakoutScoreCalculator) Name() string {
	return "Breakout"
}

func (b *BreakoutScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	count := b.Config.CandleCount
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
	res := DetectBreakout(candles, b.Config, b.CompressionScore)
	return math.Max(res.UpScore, res.DownScore), nil
}

// --- BreakoutSubscore implements Subscore for MetaScore Engine ---

// BreakoutSubscore wraps the breakout detector for MetaScore composition.
type BreakoutSubscore struct{}

func (s BreakoutSubscore) Name() string {
	return "Breakout"
}

func (s BreakoutSubscore) Compute(data interface{}, cfg interface{}) SubscoreResult {
	candles, ok := data.([]domain.Candle)
	if !ok {
		return SubscoreResult{Value: 0, Confidence: 1.0, Meta: map[string]float64{"error": 1}}
	}
	bCfg, ok := cfg.(BreakoutConfig)
	if !ok {
		bCfg = NewBreakoutConfig()
	}
	// No compression score available in subscore context — pass 0.
	res := DetectBreakout(candles, bCfg, 0)
	combined := math.Max(res.UpScore, res.DownScore)
	return SubscoreResult{
		Value:      clamp(combined, 0, 1),
		Confidence: 1.0,
		Meta: map[string]float64{
			"upScore":   res.UpScore,
			"downScore": res.DownScore,
			"bvsUp":     res.BoundaryViolationUp,
			"bvsDown":   res.BoundaryViolationDown,
			"ccsUp":     res.CloseConvictionUp,
			"ccsDown":   res.CloseConvictionDown,
			"ves":       res.VolatilityExpansion,
			"vls":       res.VolumeScore,
			"compBoost": res.CompressionBoost,
		},
	}
}
