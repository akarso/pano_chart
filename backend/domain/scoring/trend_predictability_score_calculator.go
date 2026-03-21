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
	n := series.Len()
	if n < 2 {
		return 0, fmt.Errorf("at least 2 candles required")
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
		return 0, fmt.Errorf("zero denominator in regression")
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
		return 0, nil // flat line
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
		return 0, nil // flat line
	}
	slopeNorm := b / rangeClose

	// Cluster gate: if close prices form two distinct price levels —
	// like a step function (-|_) — the movement is a regime shift, not
	// a trend.  We reuse the same gap-detection logic as the sideways-v5
	// hasDistinctClusters but require each side to hold ≥ 25% of the
	// candles, ensuring we catch real bimodal distributions (plateaus)
	// rather than false-positiving on evenly-spaced linear data.
	if closePricesClustered(closes) {
		return 0, nil
	}

	// Raw score = |slopeNorm| * R².  slopeNorm ≈ 1/(N-1) for a perfect
	// linear trend, so the raw value is typically 0..0.01 — far below
	// the 0..1 range of other calculators (sideways, gain).  Multiplying
	// by (N-1) rescales so a perfect linear trend → ~1.0, giving trend
	// a fair weight when combined with other regime scores.
	raw := math.Abs(slopeNorm) * R2
	normalised := raw * float64(n-1)
	if normalised > 1 {
		normalised = 1
	}
	return normalised, nil
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
