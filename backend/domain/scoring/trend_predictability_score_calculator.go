package scoring

import (
	"fmt"
	"math"

	"pano_chart/backend/domain"
)

// TrendPredictabilityScoreCalculator scores based on linear trend and fit.
type TrendPredictabilityScoreCalculator struct{}

func (c *TrendPredictabilityScoreCalculator) Name() string {
	return "Trend Predictability"
}

func (c *TrendPredictabilityScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	score, _, err := c.ScoreWithDirection(series)
	return score, err
}

// ScoreWithDirection is Score plus the trend's direction, derived from the
// same linear-regression slope that already determines the score's
// magnitude — it's the sign Score() computed and then discarded via
// math.Abs. bias is "up", "down", or "neutral" (flat/no-trend cases: too
// few candles, zero-variance series, or a clustered/bimodal series that
// isn't a real trend at all).
//
// This is the canonical direction source for anything that needs to know
// which way *this specific* trend score points (setup classification,
// trend-health computation) — see PR-072 for why this is kept separate
// from the market-wide aggregate-return bias used elsewhere
// (mkt.RegimeSummary.Bias / domain.EvaluationSnapshot.Bias): different
// signal, different purpose, not a substitute for each other.
func (c *TrendPredictabilityScoreCalculator) ScoreWithDirection(series domain.CandleSeries) (score float64, bias string, err error) {
	n := series.Len()
	if n < 2 {
		return 0, "neutral", fmt.Errorf("at least 2 candles required")
	}
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		candle, _ := series.At(i)
		closes[i] = candle.Close()
	}
	// Linear regression: y = a + bx
	var sumX, sumY, sumXY, sumXX float64
	for i := 0; i < n; i++ {
		sumX += float64(i)
		sumY += closes[i]
		sumXY += float64(i) * closes[i]
		sumXX += float64(i) * float64(i)
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	var num, den float64
	for i := 0; i < n; i++ {
		num += (float64(i) - meanX) * (closes[i] - meanY)
		den += (float64(i) - meanX) * (float64(i) - meanX)
	}
	if den == 0 {
		return 0, "neutral", fmt.Errorf("zero denominator in regression")
	}
	b := num / den // slope
	// R^2 goodness of fit
	var ssTot, ssRes float64
	for i := 0; i < n; i++ {
		fit := meanY + b*(float64(i)-meanX)
		ssTot += (closes[i] - meanY) * (closes[i] - meanY)
		ssRes += (closes[i] - fit) * (closes[i] - fit)
	}
	if ssTot == 0 {
		return 0, "neutral", nil // flat line
	}
	R2 := 1 - ssRes/ssTot
	// Normalize slope by price range
	minClose, maxClose := closes[0], closes[0]
	for _, v := range closes {
		if v < minClose {
			minClose = v
		}
		if v > maxClose {
			maxClose = v
		}
	}
	rangeClose := maxClose - minClose
	if rangeClose == 0 {
		return 0, "neutral", nil // flat line
	}
	slopeNorm := b / rangeClose

	bias = "neutral"
	switch {
	case slopeNorm > 0:
		bias = "up"
	case slopeNorm < 0:
		bias = "down"
	}

	// Cluster gate: if close prices form two distinct price levels —
	// like a step function (-|_) — the movement is a regime shift, not
	// a trend.  We reuse the same gap-detection logic as the sideways-v5
	// hasDistinctClusters but require each side to hold ≥ 25% of the
	// candles, ensuring we catch real bimodal distributions (plateaus)
	// rather than false-positiving on evenly-spaced linear data.
	if closePricesClustered(closes) {
		return 0, "neutral", nil
	}

	// Shape validation: penalize trends whose visual shape contradicts
	// what a human would consider a trend.  The last 15% of the chart
	// (tail) is checked strictly — a broken end kills the trend.  The
	// first 15% (head) is checked leniently — a trend that emerged from
	// a dip is acceptable.
	shapePenalty := trendShapePenalty(closes, b, minClose, maxClose)

	// Direction agreement: split the series into quarters and check
	// whether they consistently move in the same direction.  A V-shape
	// or W-shape (segments disagreeing) is not a trend.
	dirAgreement := SeriesDirectionAgreement(closes, 4)

	// Raw score = |slopeNorm| * R².  slopeNorm ≈ 1/(N-1) for a perfect
	// linear trend, so the raw value is typically 0..0.01 — far below
	// the 0..1 range of other calculators (sideways, gain).  Multiplying
	// by (N-1) rescales so a perfect linear trend → ~1.0, giving trend
	// a fair weight when combined with other regime scores.
	raw := math.Abs(slopeNorm) * R2
	normalised := raw * float64(n-1) * shapePenalty * dirAgreement
	if normalised > 1 {
		normalised = 1
	}
	return normalised, bias, nil
}

// trendShapePenalty inspects the visual shape of the chart to penalize
// trends that contradict what a human would perceive.
//
// All arithmetic is done in normalized [0,1] space (min-max scaled).
//
// For uptrends (slope > 0):
//   - TAIL (last 15%): if the lowest value drops below the chart's
//     high minus a 10% wiggle room, penalize heavily. A 50% drop from
//     the high (in normalized space) practically invalidates the trend.
//   - HEAD (first 15%): if the highest value is above the chart's low
//     plus a 10% wiggle room, penalize lightly. Markets accept trends
//     that emerged from a dip.
//
// For downtrends (slope < 0): mirror logic.
func trendShapePenalty(closes []float64, slope, minClose, maxClose float64) float64 {
	n := len(closes)
	if n < 10 {
		return 1.0 // not enough data for shape analysis
	}
	rng := maxClose - minClose
	if rng == 0 {
		return 1.0
	}

	// Normalize to [0, 1].
	norm := make([]float64, n)
	for i, v := range closes {
		norm[i] = (v - minClose) / rng
	}

	headEnd := n * 15 / 100
	if headEnd < 1 {
		headEnd = 1
	}
	tailStart := n - n*15/100
	if tailStart >= n {
		tailStart = n - 1
	}

	const wiggle = 0.20 // 20% tolerance in normalized space

	penalty := 1.0

	if slope > 0 {
		// Uptrend: tail should stay near the top.
		tailMin := norm[tailStart]
		for i := tailStart; i < n; i++ {
			if norm[i] < tailMin {
				tailMin = norm[i]
			}
		}
		// chartHigh in normalized space = 1.0.
		// Deviation = how far tailMin is below (1.0 - wiggle).
		threshold := 1.0 - wiggle // 0.90
		if tailMin < threshold {
			// Linear ramp: at threshold → penalty=1.0, at 0.40 → penalty≈0.
			drop := threshold - tailMin // 0..0.90
			// 50% drop (0.50 in normalized space) → practically zero.
			penalty *= math.Max(0.02, 1.0-drop/0.50)
		}

		// Uptrend: head allowed to start low, but extreme highs in
		// the first 15% suggest price already peaked early.
		headMax := norm[0]
		for i := 0; i < headEnd; i++ {
			if norm[i] > headMax {
				headMax = norm[i]
			}
		}
		headThreshold := 0.0 + wiggle // 0.10
		if headMax > 1.0-headThreshold {
			// Price was already near the top at the start — mild penalty.
			excess := headMax - (1.0 - headThreshold) // 0..0.90
			penalty *= math.Max(0.30, 1.0-excess*0.5)
		}
	} else {
		// Downtrend: tail should stay near the bottom.
		tailMax := norm[tailStart]
		for i := tailStart; i < n; i++ {
			if norm[i] > tailMax {
				tailMax = norm[i]
			}
		}
		threshold := wiggle // 0.10
		if tailMax > threshold {
			rise := tailMax - threshold // 0..0.90
			penalty *= math.Max(0.02, 1.0-rise/0.50)
		}

		// Downtrend: head allowed to start high, but extreme lows
		// at the start suggest the drop already happened.
		headMin := norm[0]
		for i := 0; i < headEnd; i++ {
			if norm[i] < headMin {
				headMin = norm[i]
			}
		}
		headThreshold := 1.0 - wiggle // 0.90
		if headMin < headThreshold {
			excess := headThreshold - headMin
			penalty *= math.Max(0.30, 1.0-excess*0.5)
		}
	}

	return penalty
}

// closePricesClustered detects bimodal close-price distributions (step
// functions / regime shifts).  It sorts the closes and looks for a gap
// > 10% of the total range that splits the series into two groups, each
// containing at least 25% of the candles.
func closePricesClustered(vals []float64) bool {
	n := len(vals)
	if n < 8 {
		return false
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	// sort.Float64s equivalent — insertion sort is fine for ≤ ~200 items.
	for i := 1; i < n; i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	valRange := sorted[n-1] - sorted[0]
	if valRange == 0 {
		return false
	}
	minGroupSize := n / 4 // each plateau must hold ≥ 25% of candles
	for i := 1; i < n; i++ {
		gap := sorted[i] - sorted[i-1]
		if i >= minGroupSize && (n-i) >= minGroupSize && gap > 0.10*valRange {
			return true
		}
	}
	return false
}
