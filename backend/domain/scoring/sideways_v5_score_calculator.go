package scoring

import (
	"math"
	"pano_chart/backend/domain"
)

// SidewaysV5ScoreCalculator implements SymbolScoreCalculator for v5
type SidewaysV5ScoreCalculator struct {
	Config SidewaysV5Config
}

func (s *SidewaysV5ScoreCalculator) Name() string {
	return "SidewaysAlgoV5"
}

func (s *SidewaysV5ScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	candles := make([]domain.Candle, series.Len())
	for i := 0; i < series.Len(); i++ {
		c, err := series.At(i)
		if err != nil {
			return 0, err
		}
		candles[i] = c
	}
	return DetectSidewaysV5(candles, s.Config).Score, nil
}

// DefaultSidewaysV5Config returns the default config for v5
func DefaultSidewaysV5Config() SidewaysV5Config {
	return SidewaysV5Config{
		N:             3,
		CandleCount:   110,
		IdealRangeMin: 0.005,
		IdealRangeMax: 0.02,
		ATRMultiplier: 3.0,
		W1:            1.3,
		W2:            1.2,
		W3:            1.0,
		W4:            1.0,
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
	N             int
	CandleCount   int
	IdealRangeMin float64
	IdealRangeMax float64
	ATRMultiplier float64
	W1            float64
	W2            float64
	W3            float64
	W4            float64
}

// DetectSidewaysV5 runs the structural equilibrium detector
// Returns SidewaysResult
func DetectSidewaysV5(candles []domain.Candle, cfg SidewaysV5Config) SidewaysResult {
	if len(candles) < cfg.CandleCount {
		println("[SidewaysV5] Not enough candles: ", len(candles), " required:", cfg.CandleCount)
		return SidewaysResult{Score: 0, Components: map[string]float64{"CCS": 0, "OQS": 0, "DCS": 0, "VOS": 0, "SRM": 1}, SpikeDetected: false}
	}

	println("[SidewaysV5] Running with ", len(candles), " candles, config.CandleCount=", cfg.CandleCount)

	// --- 1. Detect local extrema ---
	highsIdx, lowsIdx, extremaIdx := detectExtrema(candles, cfg.N)
	println("[SidewaysV5] Extrema count:", len(extremaIdx))

	// --- 2. Fit regression lines to all extrema ---
	extremaCandles := make([]domain.Candle, len(extremaIdx))
	for i, idx := range extremaIdx {
		extremaCandles[i] = candles[idx]
	}
	println("[SidewaysV5] ExtremaCandles:", len(extremaCandles))
	upperSlope, upperIntercept := linearRegression(extremaCandles)
	lowerSlope, lowerIntercept := linearRegression(extremaCandles)
	println("[SidewaysV5] upperSlope=", upperSlope, " lowerSlope=", lowerSlope)

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
	println("[SidewaysV5] CCS=", CCS, " parallelScore=", parallelScore, " deviationScore=", deviationScore, " widthStabilityScore=", widthStabilityScore)

	// --- 4. Compute OQS ---
	altScore := alternationScore(extremaIdx)
	evennessScoreVal := evennessScore(extremaIdx, candles)
	brScore := boundaryRespectScoreV5(candles, extremaIdx, upperSlope, upperIntercept, lowerSlope, lowerIntercept)
	OQS := clamp((altScore+evennessScoreVal+brScore)/3, 0, 1)
	println("[SidewaysV5] OQS=", OQS, " altScore=", altScore, " evennessScore=", evennessScoreVal, " brScore=", brScore)

	// --- 5. Compute DCS ---
	fullSlope, _ := linearRegression(candles)
	channelWidth := mean(widths)
	normSlope := math.Abs(fullSlope) / (channelWidth + 1e-6)
	DCS := 1 - clamp(normSlope, 0, 1)
	println("[SidewaysV5] DCS=", DCS, " fullSlope=", fullSlope, " channelWidth=", channelWidth, " normSlope=", normSlope)

	// --- 6. Compute VOS ---
	maxHigh, minLow, meanPrice := extremesAndMean(candles)
	rangePercent := (maxHigh - minLow) / (meanPrice + 1e-6)
	atr := averageATR(candles)
	// atrRatio := atr / (meanPrice + 1e-6) // unused
	vosRaw := bellShapedScore(rangePercent, cfg.IdealRangeMin, cfg.IdealRangeMax)
	VOS := clamp(vosRaw/0.33, 0, 1)
	println("[SidewaysV5] VOS=", VOS, " vosRaw=", vosRaw, " rangePercent=", rangePercent, " atr=", atr)

	// --- 7. Spike detection ---
	spikeIdx, spikeDetected := detectSpike(candles, atr, cfg.ATRMultiplier)
	println("[SidewaysV5] spikeDetected=", spikeDetected, " spikeIdx=", spikeIdx)

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
		println("[SidewaysV5] Recovery: structurePre=", structurePre, " structurePost=", structurePost, " recoveryScore=", recoveryScore)
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

	// --- 9. Final composition (HybridStructural) ---
	structureCore := math.Pow(CCS, cfg.W1) * math.Pow(OQS, cfg.W2)
	driftAdj := math.Pow(DCS, cfg.W3)
	volAdj := math.Pow(VOS, cfg.W4)
	baseScore := structureCore * driftAdj * volAdj
	finalScore := baseScore * SRM
	finalScore = clamp(finalScore, 0, 1)
	println("[SidewaysV5] FINAL: finalScore=", finalScore, " structureCore=", structureCore, " driftAdj=", driftAdj, " volAdj=", volAdj, " SRM=", SRM)

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
	println("[SidewaysV5] Extrema sequence:")
	for _, idx := range all {
		label := ""
		if contains(highs, idx) {
			label += "H"
		}
		if contains(lows, idx) {
			label += "L"
		}
		println("  idx=", idx, " type=", label, " high=", candles[idx].High(), " low=", candles[idx].Low())
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
		println("[evennessScore] idx1=", extrema[i-1], " idx2=", extrema[i], " t1=", t1.String(), " t2=", t2.String(), " delta=", deltas[i-1])
	}
	std := stddev(deltas)
	meanDelta := mean(deltas)
	println("[evennessScore] deltas=", deltas, " stddev=", std, " mean=", meanDelta)
	k := 5000.0
	return math.Exp(-std / k)
}

func boundaryRespectScoreV5(candles []domain.Candle, extrema []int, us, ui, ls, li float64) float64 {
	if len(extrema) == 0 {
		return 1
	}
	var sum float64
	for _, idx := range extrema {
		y := (candles[idx].High() + candles[idx].Low()) / 2
		upper := us*float64(idx) + ui
		lower := ls*float64(idx) + li
		dist := math.Min(math.Abs(y-upper), math.Abs(y-lower))
		sum += dist
	}
	meanDist := sum / float64(len(extrema))
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
	println("[SidewaysV5] Extrema count:", len(extremaIdx))

	// --- 2. Fit regression lines to all extrema ---
	extremaCandles := make([]domain.Candle, len(extremaIdx))
	for i, idx := range extremaIdx {
		extremaCandles[i] = candles[idx]
	}
	println("[SidewaysV5] ExtremaCandles:", len(extremaCandles))
	upperSlope, upperIntercept := linearRegression(extremaCandles)
	lowerSlope, lowerIntercept := linearRegression(extremaCandles)
	println("[SidewaysV5] upperSlope=", upperSlope, " lowerSlope=", lowerSlope)

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
	brScore := boundaryRespectScoreV5(candles, extremaIdx, upperSlope, upperIntercept, lowerSlope, lowerIntercept)
	OQS := clamp((altScore+evennessScoreVal+brScore)/3, 0, 1)
	println("[SidewaysV5] OQS=", OQS, " altScore=", altScore, " evennessScore=", evennessScoreVal, " brScore=", brScore)

	fullSlope, _ := linearRegression(candles)
	channelWidth := mean(widths)
	normSlope := math.Abs(fullSlope) / (channelWidth + 1e-6)
	DCS := 1 - clamp(normSlope, 0, 1)
	return CCS, OQS, DCS
}

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
	return SubscoreResult{
		Value:      clamp(res.Score, 0, 1),
		Confidence: 1.0,
		Meta:       res.Components,
	}
}
