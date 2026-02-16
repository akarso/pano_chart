package scoring

import (
	"fmt"
	"math"
	"sort"

	"pano_chart/backend/domain"
)

// SidewaysV2ScoreCalculator scores structural sideways (channel) behavior.
// It detects clear horizontal boundaries, repeated band interaction,
// limited slope bias, and reasonable channel width.
type SidewaysV2ScoreCalculator struct{}

func (c *SidewaysV2ScoreCalculator) Name() string {
	return "Sideways Consistency"
}

func (c *SidewaysV2ScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	n := series.Len()
	if n < 6 {
		return 0, fmt.Errorf("at least 6 candles required")
	}

	closes := extractCloses(series)

	minPrice, maxPrice, meanPrice := priceStats(closes)
	rangePrice := maxPrice - minPrice
	if rangePrice == 0 {
		return 0, nil // flat line
	}

	fs := flatnessScore(closes, rangePrice)
	cts := channelTightnessScore(rangePrice, meanPrice)
	brs := boundaryRespectScore(closes, rangePrice)
	sns := slopeNeutralityScore(closes, rangePrice)

	score := 0.35*fs + 0.25*brs + 0.20*sns + 0.20*cts

	// Noise penalty: if ODS > 0.8, reduce score by 15%
	ods := oscillationDensity(closes)
	if ods > 0.8 {
		score *= 0.85
	}

	return clamp01(score), nil
}

// extractCloses returns a slice of close prices from the series.
func extractCloses(series domain.CandleSeries) []float64 {
	n := series.Len()
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		candle, _ := series.At(i)
		closes[i] = candle.Close()
	}
	return closes
}

// priceStats returns min, max, and mean of the given prices.
func priceStats(closes []float64) (min, max, mean float64) {
	min, max = closes[0], closes[0]
	sum := 0.0
	for _, v := range closes {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}
	mean = sum / float64(len(closes))
	return
}

// flatnessScore computes FS = 1 - NDR (net displacement ratio).
func flatnessScore(closes []float64, rangePrice float64) float64 {
	ndr := math.Abs(closes[len(closes)-1]-closes[0]) / rangePrice
	return clamp01(1 - ndr)
}

// channelTightnessScore penalizes channels that are too narrow or too wide.
// Ideal mid: 5%, tolerance: 3%.
func channelTightnessScore(rangePrice, meanPrice float64) float64 {
	if meanPrice == 0 {
		return 0
	}
	const idealMid = 0.05
	const tolerance = 0.03

	normalizedRange := rangePrice / meanPrice
	cts := 1 - math.Abs(normalizedRange-idealMid)/tolerance
	return clamp01(cts)
}

// boundaryRespectScore checks that price repeatedly touches both upper and lower bands.
// upper_band = 90th percentile, lower_band = 10th percentile.
func boundaryRespectScore(closes []float64, rangePrice float64) float64 {
	n := len(closes)

	sorted := make([]float64, n)
	copy(sorted, closes)
	sort.Float64s(sorted)

	upperBand := percentile(sorted, 0.90)
	lowerBand := percentile(sorted, 0.10)
	epsilon := 0.10 * rangePrice

	touchUpper := 0
	touchLower := 0
	for _, c := range closes {
		if c >= upperBand-epsilon {
			touchUpper++
		}
		if c <= lowerBand+epsilon {
			touchLower++
		}
	}

	// expected_touch_count: at least 10% of candles should touch each band
	expectedTouches := max(1, n/10)

	brs := float64(min(touchUpper, touchLower)) / float64(expectedTouches)
	return clamp01(brs)
}

// slopeNeutralityScore penalizes directional drift via linear regression slope.
func slopeNeutralityScore(closes []float64, rangePrice float64) float64 {
	n := len(closes)
	if rangePrice == 0 || n < 2 {
		return 0
	}

	slope := linearRegressionSlope(closes)
	normalizedSlope := math.Abs(slope) * float64(n) / rangePrice
	return clamp01(1 - normalizedSlope)
}

// linearRegressionSlope computes the slope of a linear fit over indexed values.
func linearRegressionSlope(values []float64) float64 {
	n := float64(len(values))
	var sumX, sumY, sumXY, sumXX float64
	for i, v := range values {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
	}
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

// oscillationDensity counts local extrema as a fraction of possible positions.
func oscillationDensity(closes []float64) float64 {
	n := len(closes)
	if n <= 2 {
		return 0
	}
	extrema := 0
	for i := 1; i < n-1; i++ {
		if (closes[i] > closes[i-1] && closes[i] > closes[i+1]) ||
			(closes[i] < closes[i-1] && closes[i] < closes[i+1]) {
			extrema++
		}
	}
	return float64(extrema) / float64(n-2)
}

// percentile returns the value at the given percentile p (0–1) from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := p * float64(n-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= n {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// clamp01 constrains a value to [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
