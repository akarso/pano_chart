package scoring

import (
	"fmt"
	"math"
	"os"
	"pano_chart/backend/domain"
)

// SidewaysV5ScoreCalculator implements SymbolScoreCalculator for v5
type SidewaysV5ScoreCalculator struct {
	Config SidewaysV5Config
}

func (s *SidewaysV5ScoreCalculator) Name() string {
	return "Sideways Consistency"
}

func (s *SidewaysV5ScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	count := s.Config.CandleCount
	length := series.Len()
	start := 0
	if length > count {
		start = length - count
		length = count
	}
	candles := make([]domain.Candle, length)
	for i := 0; i < length; i++ {
		c, err := series.At(start + i)
		if err != nil {
			return 0, err
		}
		candles[i] = c
	}
	sc := DetectSidewaysV5(candles, s.Config).Score
	fmt.Fprintf(os.Stderr, "[SidewaysV5] score 1: %+v\n", sc)
	return sc, nil
	//return DetectSidewaysV5(candles, s.Config).Score, nil
}

// DefaultSidewaysV5Config returns the config for v5, loading from .env if present.
// Each value can be overridden by an environment variable in .env or the process env.
// Example .env:
//
//	N=3
//	CANDLE_COUNT=110
//	IDEAL_RANGE_MIN=0.005
//	IDEAL_RANGE_MAX=0.02
//	ATR_MULTIPLIER=3.0
//	W1=1.0
//	W2=2.0
//	W3=1.0
//	W4=1.0
func DefaultSidewaysV5Config() SidewaysV5Config {
	LoadEnv()

	return SidewaysV5Config{
		N:             EnvInt("SIDEWAYS_N", 3),
		CandleCount:   EnvInt("SIDEWAYS_CANDLE_COUNT", 110),
		IdealRangeMin: EnvFloat("SIDEWAYS_IDEAL_RANGE_MIN", 0.005),
		IdealRangeMax: EnvFloat("SIDEWAYS_IDEAL_RANGE_MAX", 0.02),
		ATRMultiplier: EnvFloat("SIDEWAYS_ATR_MULTIPLIER", 3.0),
		W1:            EnvFloat("SIDEWAYS_W1", 1.0),
		W2:            EnvFloat("SIDEWAYS_W2", 2.0),
		W3:            EnvFloat("SIDEWAYS_W3", 1.0),
		W4:            EnvFloat("SIDEWAYS_W4", 1.0),
		ExtremaCount:  EnvInt("SIDEWAYS_EXTREMA_COUNT", 8),
	}
}

// SidewaysResult is the output struct for Sideways v5
// Compatible with MetaScore Engine
// All components normalized [0,1]
type SidewaysResult struct {
	Score         float64
	Components    map[string]float64
	SpikeDetected bool
}

// SidewaysV5Config holds all tunable parameters for Sideways v5
// (weights, N, candleCount, ideal volatility band, ATR multiplier)
type SidewaysV5Config struct {
	N             int     // Extrema window size
	CandleCount   int     // Number of candles to analyze
	IdealRangeMin float64 // Min ideal volatility (fraction)
	IdealRangeMax float64 // Max ideal volatility (fraction)
	ATRMultiplier float64 // Spike detection multiplier
	W1            float64 // Weight for CCS (channel structure)
	W2            float64 // Weight for OQS (oscillation quality)
	W3            float64 // Weight for DCS (drift control)
	W4            float64 // Weight for VOS (volatility/oscillation)
	ExtremaCount  int     // Minimum number of extrema required
}

// DetectSidewaysV5 runs the structural equilibrium detector
// Returns SidewaysResult
func DetectSidewaysV5(candles []domain.Candle, cfg SidewaysV5Config) SidewaysResult {
	// --- 0. Moving average flatness filter ---
	// ma := simpleMovingAverage(candles, 50) // window size 40, can be tuned
	// relEps := 0.001 * mean(ma)             // 0.1% of mean MA value
	// if !isMAFlat(ma, relEps) {
	//  println("[SidewaysV5] MA not strictly flat (relEps=", relEps, "), skipping scoring.")
	//  return SidewaysResult{Score: 0, Components: map[string]float64{"CCS": 0, "OQS": 0, "DCS": 0, "VOS": 0, "SRM": 1}, SpikeDetected: false}
	// }
	// isMAFlat returns true if all MA values are within epsilon of the mean

	// simpleMovingAverage computes SMA for candle closes with given window size

	// --- 0. Initial extrema clustering filter ---
	// Log config struct

	highsIdx, lowsIdx, extremaIdx := detectExtrema(candles, cfg.N)
	// Gather all extrema values
	extremaVals := make([]float64, len(extremaIdx))
	for i, idx := range extremaIdx {
		extremaVals[i] = (candles[idx].High() + candles[idx].Low()) / 2
	}
	// Filter: require at least cfg.ExtremaCount extrema
	if len(extremaIdx) < cfg.ExtremaCount {
		return SidewaysResult{Score: 0, Components: map[string]float64{"CCS": 0, "OQS": 0, "DCS": 0, "VOS": 0, "SRM": 1}, SpikeDetected: false}
	}

	// --- 0b. Minimum range gate: reject micro-flat noise ---
	// If the price range is far below the ideal minimum, the data is too flat
	// to be structurally meaningful as sideways movement.
	maxH, minL, meanP := extremesAndMean(candles)
	rangeP := (maxH - minL) / (meanP + 1e-6)
	if rangeP < cfg.IdealRangeMin*0.5 {
		return SidewaysResult{Score: 0, Components: map[string]float64{"CCS": 0, "OQS": 0, "DCS": 0, "VOS": 0, "SRM": 1}, SpikeDetected: false}
	}

	// --- 1. Detect local extrema ---
	// (already done above)

	// --- 2. Fit regression lines to all extrema ---
	extremaCandles := make([]domain.Candle, len(extremaIdx))
	for i, idx := range extremaIdx {
		extremaCandles[i] = candles[idx]
	}
	upperSlope, upperIntercept := linearRegression(extremaCandles)
	lowerSlope, lowerIntercept := linearRegression(extremaCandles)

	// --- 3. Compute CCS ---
	peaks := make([]domain.Candle, len(highsIdx))
	for i, idx := range highsIdx {
		peaks[i] = candles[idx]
	}
	troughs := make([]domain.Candle, len(lowsIdx))
	for i, idx := range lowsIdx {
		troughs[i] = candles[idx]
	}
	widths := channelWidths(peaks, troughs)
	parallelScore := 1 - math.Abs(upperSlope-lowerSlope)/0.1 // slopeNormalization=0.1
	parallelScore = clamp(parallelScore, 0, 1)
	deviationScore := 1 - stddevFromLine(extremaCandles, upperSlope, upperIntercept)/1.0 // 1.0 normalization
	deviationScore = clamp(deviationScore, 0, 1)
	widthStabilityScore := 1 - stddev(widths)/1.0
	widthStabilityScore = clamp(widthStabilityScore, 0, 1)
	CCS := clamp((parallelScore+deviationScore+widthStabilityScore)/3, 0, 1)

	// --- 4. Compute OQS ---
	altScore := alternationScore(extremaIdx)
	evennessScoreVal := evennessScore(extremaIdx, candles)
	brScore := boundaryRespectScoreV5(candles, highsIdx, lowsIdx, upperSlope, upperIntercept, lowerSlope, lowerIntercept)
	OQS := clamp((altScore+evennessScoreVal+brScore)/3, 0, 1)

	// --- 5. Compute DCS ---
	fullSlope, _ := linearRegression(candles)
	channelWidth := mean(widths)
	normSlope := math.Abs(fullSlope) / (channelWidth + 1e-6)
	DCS := 1 - clamp(normSlope, 0, 1)

	// --- 6. Compute VOS ---
	maxHigh, minLow, meanPrice := extremesAndMean(candles)
	rangePercent := (maxHigh - minLow) / (meanPrice + 1e-6)
	atr := averageATR(candles)
	// atrRatio := atr / (meanPrice + 1e-6) // unused
	vosRaw := bellShapedScore(rangePercent, cfg.IdealRangeMin, cfg.IdealRangeMax)
	VOS := clamp(vosRaw/0.33, 0, 1)

	// --- 7. Spike detection ---
	spikeIdx, spikeDetected := detectSpike(candles, atr, cfg.ATRMultiplier)

	// --- 8. Recovery evaluation ---
	SRM := 1.0
	if spikeDetected {
		pre := candles[:spikeIdx]
		post := candles[spikeIdx+1:]
		preCCS, preOQS, preDCS := quickStructure(pre, cfg)
		postCCS, postOQS, postDCS := quickStructure(post, cfg)
		structurePre := min3(preCCS, preOQS, preDCS)
		structurePost := min3(postCCS, postOQS, postDCS)
		recoveryScore := math.Min(structurePre, structurePost)
		if recoveryScore > 0.8 {
			SRM = 0.9
		} else if recoveryScore > 0.5 {
			SRM = 0.7
		} else if recoveryScore > 0.3 {
			SRM = 0.5
		} else {
			SRM = 0.3
		}
	}

	// --- 9. Final composition (Weighted Average) ---
	fmt.Fprintf(os.Stderr, "[SidewaysV5] cfg:: %+v\n", cfg)
	weightedSum := cfg.W1*CCS + cfg.W2*OQS + cfg.W3*DCS + cfg.W4*VOS
	totalWeight := cfg.W1 + cfg.W2 + cfg.W3 + cfg.W4
	avgScore := 0.0
	fmt.Fprintf(os.Stderr, "[SidewaysV5] totalWeight: %+v\n", totalWeight)
	if totalWeight > 0 {
		avgScore = weightedSum / totalWeight
	}
	finalScore := avgScore * SRM
	finalScore = clamp(finalScore, 0, 1)

	return SidewaysResult{
		Score: finalScore,
		Components: map[string]float64{
			"CCS": CCS,
			"OQS": OQS,
			"DCS": DCS,
			"VOS": VOS,
			"SRM": SRM,
		},
		SpikeDetected: spikeDetected,
	}
}

// --- Helper functions for Sideways v5 ---
func detectExtrema(candles []domain.Candle, N int) (highs, lows, all []int) {
	epsilon := 1e-8
	lastType := ""
	for i := N; i < len(candles)-N; i++ {
		isHigh := true
		isLow := true
		for j := 1; j <= N; j++ {
			if candles[i].High() < candles[i-j].High()-epsilon || candles[i].High() < candles[i+j].High()-epsilon {
				isHigh = false
			}
			if candles[i].Low() > candles[i-j].Low()+epsilon || candles[i].Low() > candles[i+j].Low()+epsilon {
				isLow = false
			}
		}
		if isHigh && lastType != "H" {
			highs = append(highs, i)
			all = append(all, i)
			lastType = "H"
			continue
		}
		if isLow && lastType != "L" {
			lows = append(lows, i)
			all = append(all, i)
			lastType = "L"
		}
	}
	// Sanity check: print sequence of high/low types for all
	for _, idx := range all {
		label := ""
		if contains(highs, idx) {
			label += "H"
		}
		if contains(lows, idx) {
			label += "L"
		}
	}
	return highs, lows, all
}

func contains(arr []int, v int) bool {
	for _, x := range arr {
		if x == v {
			return true
		}
	}
	return false
}

// func splitExtrema(candles []domain.Candle, extrema []int) ([]domain.Candle, []domain.Candle) {
// 	var peaks, troughs []domain.Candle
// 	var peakIdxs, troughIdxs []int
// 	for _, idx := range extrema {
// 		// Only classify if not at boundary
// 		if idx > 0 && idx < len(candles)-1 {
// 			prev := candles[idx-1]
// 			next := candles[idx+1]
// 			c := candles[idx]
// 			// Peak: High is greater than both neighbors
// 			if c.High() > prev.High() && c.High() > next.High() {
// 				peaks = append(peaks, c)
// 				peakIdxs = append(peakIdxs, idx)
// 			}
// 			// Trough: Low is less than both neighbors
// 			if c.Low() < prev.Low() && c.Low() < next.Low() {
// 				troughs = append(troughs, c)
// 				troughIdxs = append(troughIdxs, idx)
// 			}
// 		}
// 	}
// 	println("[SidewaysV5] splitExtrema: peakIdxs=", fmtIntSlice(peakIdxs), "troughIdxs=", fmtIntSlice(troughIdxs))
// 	return peaks, troughs
// }

// Helper to format int slices for logging
// func fmtIntSlice(s []int) string {
// 	if len(s) == 0 {
// 		return "[]"
// 	}
// 	out := "["
// 	for i, v := range s {
// 		if i > 0 {
// 			out += ", "
// 		}
// 		out += fmt.Sprintf("%d", v)
// 	}
// 	out += "]"
// 	return out
// }

func linearRegression(candles []domain.Candle) (slope, intercept float64) {
	if len(candles) < 2 {
		return 0, 0
	}
	n := float64(len(candles))
	var sumX, sumY, sumXY, sumXX float64
	for i, c := range candles {
		x := float64(i)
		y := (c.High() + c.Low()) / 2
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	slope = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX + 1e-6)
	intercept = (sumY - slope*sumX) / n
	return
}

func stddevFromLine(candles []domain.Candle, slope, intercept float64) float64 {
	if len(candles) == 0 {
		return 1
	}
	var sum, mean float64
	for i, c := range candles {
		x := float64(i)
		y := (c.High() + c.Low()) / 2
		dist := math.Abs(y - (slope*x + intercept))
		sum += dist
	}
	mean = sum / float64(len(candles))
	var variance float64
	for i, c := range candles {
		x := float64(i)
		y := (c.High() + c.Low()) / 2
		dist := math.Abs(y - (slope*x + intercept))
		variance += (dist - mean) * (dist - mean)
	}
	return math.Sqrt(variance / float64(len(candles)))
}

func channelWidths(peaks, troughs []domain.Candle) []float64 {
	minLen := len(peaks)
	if len(troughs) < minLen {
		minLen = len(troughs)
	}
	widths := make([]float64, minLen)
	for i := 0; i < minLen; i++ {
		widths[i] = math.Abs((peaks[i].High()+peaks[i].Low())/2 - (troughs[i].High()+troughs[i].Low())/2)
	}
	return widths
}

func stddev(vals []float64) float64 {
	if len(vals) == 0 {
		return 1
	}
	mean := mean(vals)
	var sum float64
	for _, v := range vals {
		sum += (v - mean) * (v - mean)
	}
	return math.Sqrt(sum / float64(len(vals)))
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func alternationScore(extrema []int) float64 {
	return 1
	// if len(extrema) < 2 {
	// 	return 1
	// }
	// penalty := 0.0
	// for i := 2; i < len(extrema); i++ {
	// 	delta1 := extrema[i-1] - extrema[i-2]
	// 	delta2 := extrema[i] - extrema[i-1]
	// 	if (delta1 > 0 && delta2 > 0) || (delta1 < 0 && delta2 < 0) {
	// 		penalty += 1
	// 	}
	// }
	// k := 2.0 // scaling factor for exponential decay
	// return math.Exp(-penalty / k)
}

func evennessScore(extrema []int, candles []domain.Candle) float64 {
	if len(extrema) < 2 {
		return 1
	}
	deltas := make([]float64, len(extrema)-1)
	for i := 1; i < len(extrema); i++ {
		t1 := candles[extrema[i-1]].Timestamp()
		t2 := candles[extrema[i]].Timestamp()
		deltas[i-1] = float64(t2.Sub(t1).Seconds())
	}
	std := stddev(deltas)
	//meanDelta := mean(deltas)
	k := 5000.0
	return math.Exp(-std / k)
}

func boundaryRespectScoreV5(candles []domain.Candle, highsIdx, lowsIdx []int, us, ui, ls, li float64) float64 {
	if len(highsIdx) == 0 && len(lowsIdx) == 0 {
		return 1
	}
	var highSum float64
	for _, idx := range highsIdx {
		priceHigh := candles[idx].High()
		upper := us*float64(idx) + ui
		dist := math.Abs(priceHigh - upper)
		highSum += dist
	}
	var lowSum float64
	for _, idx := range lowsIdx {
		priceLow := candles[idx].Low()
		lower := ls*float64(idx) + li
		dist := math.Abs(priceLow - lower)
		lowSum += dist
	}
	meanHigh := 0.0
	meanLow := 0.0
	if len(highsIdx) > 0 {
		meanHigh = highSum / float64(len(highsIdx))
	}
	if len(lowsIdx) > 0 {
		meanLow = lowSum / float64(len(lowsIdx))
	}
	meanDist := (meanHigh + meanLow) / 2
	k := 2.0 // scaling factor for exponential decay
	return math.Exp(-meanDist / k)
}

func extremesAndMean(candles []domain.Candle) (maxHigh, minLow, meanPrice float64) {
	if len(candles) == 0 {
		return 0, 0, 0
	}
	maxHigh = candles[0].High()
	minLow = candles[0].Low()
	sum := 0.0
	for _, c := range candles {
		if c.High() > maxHigh {
			maxHigh = c.High()
		}
		if c.Low() < minLow {
			minLow = c.Low()
		}
		sum += (c.High() + c.Low()) / 2
	}
	meanPrice = sum / float64(len(candles))
	return
}

func averageATR(candles []domain.Candle) float64 {
	if len(candles) < 2 {
		return 0
	}
	sum := 0.0
	for i := 1; i < len(candles); i++ {
		r1 := candles[i].High() - candles[i].Low()
		r2 := math.Abs(candles[i].High() - candles[i-1].Close())
		r3 := math.Abs(candles[i].Low() - candles[i-1].Close())
		tr := math.Max(r1, math.Max(r2, r3))
		sum += tr
	}
	return sum / float64(len(candles)-1)
}

func bellShapedScore(val, idealMin, idealMax float64) float64 {
	if val < idealMin {
		return clamp((val/idealMin)*0.5, 0, 1)
	}
	if val > idealMax {
		return clamp((idealMax/val)*0.5, 0, 1)
	}
	return 1.0
}

func detectSpike(candles []domain.Candle, atr, k float64) (int, bool) {
	for i, c := range candles {
		if (c.High()-c.Low()) > k*atr && atr > 0 {
			return i, true
		}
	}
	return -1, false
}

func quickStructure(candles []domain.Candle, cfg SidewaysV5Config) (float64, float64, float64) {
	highsIdx, lowsIdx, extremaIdx := detectExtrema(candles, cfg.N)

	// --- 2. Fit regression lines to all extrema ---
	extremaCandles := make([]domain.Candle, len(extremaIdx))
	for i, idx := range extremaIdx {
		extremaCandles[i] = candles[idx]
	}
	upperSlope, upperIntercept := linearRegression(extremaCandles)
	lowerSlope, lowerIntercept := linearRegression(extremaCandles)

	peaks := make([]domain.Candle, len(highsIdx))
	for i, idx := range highsIdx {
		peaks[i] = candles[idx]
	}
	troughs := make([]domain.Candle, len(lowsIdx))
	for i, idx := range lowsIdx {
		troughs[i] = candles[idx]
	}
	widths := channelWidths(peaks, troughs)
	parallelScore := 1 - math.Abs(upperSlope-lowerSlope)/0.1
	parallelScore = clamp(parallelScore, 0, 1)
	deviationScore := 1 - stddevFromLine(extremaCandles, upperSlope, upperIntercept)/1.0
	deviationScore = clamp(deviationScore, 0, 1)
	widthStabilityScore := 1 - stddev(widths)/1.0
	widthStabilityScore = clamp(widthStabilityScore, 0, 1)
	CCS := clamp((parallelScore+deviationScore+widthStabilityScore)/3, 0, 1)
	altScore := alternationScore(extremaIdx)
	evennessScoreVal := evennessScore(extremaIdx, candles)
	brScore := boundaryRespectScoreV5(candles, highsIdx, lowsIdx, upperSlope, upperIntercept, lowerSlope, lowerIntercept)
	OQS := clamp((altScore+evennessScoreVal+brScore)/3, 0, 1)

	fullSlope, _ := linearRegression(candles)
	channelWidth := mean(widths)
	normSlope := math.Abs(fullSlope) / (channelWidth + 1e-6)
	DCS := 1 - clamp(normSlope, 0, 1)
	return CCS, OQS, DCS
}

// Note to AI agents: The following helper functions (simpleMovingAverage, hasDistinctClusters, isMAFlat) were part of an initial idea to pre-filter for MA flatness and distinct clusters of extrema, but were found to be too strict and are currently not used in the main detection flow. They can be re-enabled or refined in the future if needed.
// func simpleMovingAverage(candles []domain.Candle, window int) []float64 {
// 	n := len(candles)
// 	if n < window {
// 		return nil
// 	}
// 	ma := make([]float64, n-window+1)
// 	for i := 0; i <= n-window; i++ {
// 		sum := 0.0
// 		for j := 0; j < window; j++ {
// 			sum += candles[i+j].Close()
// 		}
// 		ma[i] = sum / float64(window)
// 	}
// 	return ma
// }

// // hasDistinctClusters checks if extrema values form two distinct clusters (simple threshold-based method)
// func hasDistinctClusters(vals []float64) bool {
// 	if len(vals) < 4 {
// 		return false // not enough points to form clusters
// 	}
// 	// Sort values
// 	sorted := make([]float64, len(vals))
// 	copy(sorted, vals)
// 	// Simple insertion sort (small arrays)
// 	for i := 1; i < len(sorted); i++ {
// 		key := sorted[i]
// 		j := i - 1
// 		for j >= 0 && sorted[j] > key {
// 			sorted[j+1] = sorted[j]
// 			j--
// 		}
// 		sorted[j+1] = key
// 	}
// 	// Find largest gap between consecutive values
// 	maxGap := 0.0
// 	maxIdx := 0
// 	for i := 1; i < len(sorted); i++ {
// 		gap := sorted[i] - sorted[i-1]
// 		if gap > maxGap {
// 			maxGap = gap
// 			maxIdx = i
// 		}
// 	}
// 	// Heuristic: if the largest gap splits the data into two groups, and each group has at least 2 points, and the gap is at least 10% of the value range, call it two clusters
// 	n1 := maxIdx
// 	n2 := len(sorted) - maxIdx
// 	valRange := sorted[len(sorted)-1] - sorted[0]
// 	if n1 >= 2 && n2 >= 2 && maxGap > 0.1*valRange {
// 		return true
// 	}
// 	return false
// }

// func isMAFlat(ma []float64, epsilon float64) bool {
// 	if len(ma) == 0 {
// 		return false
// 	}
// 	m := mean(ma)
// 	for _, v := range ma {
// 		if math.Abs(v-m) > epsilon {
// 			return false
// 		}
// 	}
// 	return true
// }

func min3(a, b, c float64) float64 {
	return math.Min(a, math.Min(b, c))
}

// Utility: clamp value to [min, max]
func clamp(val, min, max float64) float64 {
	return math.Max(min, math.Min(val, max))
}

// SidewaysV5Subscore implements Subscore interface for MetaScore Engine
type SidewaysV5Subscore struct {
}

func (s SidewaysV5Subscore) Name() string {
	return "SidewaysV5"
}

// Compute runs Sideways v5 detector and returns SubscoreResult
func (s SidewaysV5Subscore) Compute(data interface{}, cfg interface{}) SubscoreResult {
	candles, ok := data.([]domain.Candle)
	if !ok {
		return SubscoreResult{Value: 0, Confidence: 1.0, Meta: map[string]float64{"error": 1}}
	}
	v5cfg, ok := cfg.(SidewaysV5Config)
	if !ok {
		v5cfg = SidewaysV5Config{N: 3, CandleCount: 110, IdealRangeMin: 0.005, IdealRangeMax: 0.02, ATRMultiplier: 3.0, W1: 1.3, W2: 1.2, W3: 1.0, W4: 1.0}
	}
	res := DetectSidewaysV5(candles, v5cfg)
	fmt.Fprintf(os.Stderr, "[SidewaysV5] score 2: %+v\n", res.Score)
	return SubscoreResult{
		Value:      clamp(res.Score, 0, 1),
		Confidence: 1.0,
		Meta:       res.Components,
	}
}
