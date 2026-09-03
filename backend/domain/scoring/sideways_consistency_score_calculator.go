package scoring

import (
	"pano_chart/backend/domain"
	"fmt"
	"math"
)

// SidewaysConsistencyScoreCalculator scores range-bound, oscillatory behavior.
type SidewaysConsistencyScoreCalculator struct{}

func (c *SidewaysConsistencyScoreCalculator) Name() string {
	return "Sideways Consistency"
}

func (c *SidewaysConsistencyScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	n := series.Len()
	if n < 6 {
		return 0, fmt.Errorf("at least 6 candles required")
	}
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		candle, _ := series.At(i)
		closes[i] = candle.Close()
	}
	// --- 1. Net Displacement Ratio (NDR) ---
	p0 := closes[0]
	pn := closes[n-1]
	minPrice, maxPrice := closes[0], closes[0]
	for _, v := range closes {
		if v < minPrice {
			minPrice = v
		}
		if v > maxPrice {
			maxPrice = v
		}
	}
	rangePrice := maxPrice - minPrice
	var ndr float64
	if rangePrice == 0 {
		// Flat line, per spec: score = 0
		return 0, nil
	} else {
		ndr = math.Abs(pn-p0) / rangePrice
	}
	if ndr < 0 {
		ndr = 0
	}
	if ndr > 1 {
		ndr = 1
	}
	// --- 2. Range Stability Score (RSS) ---
	window := 5
	if n < window {
		window = n
	}
	windowRanges := make([]float64, n-window+1)
	for i := 0; i <= n-window; i++ {
		wMin, wMax := closes[i], closes[i]
		for j := i; j < i+window; j++ {
			if closes[j] < wMin {
				wMin = closes[j]
			}
			if closes[j] > wMax {
				wMax = closes[j]
			}
		}
		windowRanges[i] = wMax - wMin
	}
	var sum, mean float64
	for _, v := range windowRanges {
		sum += v
	}
	mean = sum / float64(len(windowRanges))
	var stddev float64
	for _, v := range windowRanges {
		stddev += (v - mean) * (v - mean)
	}
	stddev = math.Sqrt(stddev / float64(len(windowRanges)))
	rss := 0.0
	if mean > 0 {
		rss = 1 - (stddev / mean)
	}
	if rss < 0 {
		rss = 0
	}
	if rss > 1 {
		rss = 1
	}
	// --- 3. Oscillation Density Score (ODS) ---
	extrema := 0
	for i := 1; i < n-1; i++ {
		if (closes[i] > closes[i-1] && closes[i] > closes[i+1]) || (closes[i] < closes[i-1] && closes[i] < closes[i+1]) {
			extrema++
		}
	}
	ods := 0.0
	if n > 2 {
		ods = float64(extrema) / float64(n-2)
	}
	if ods < 0 {
		ods = 0
	}
	if ods > 1 {
		ods = 1
	}
	// --- Final Score ---
	score := (1 - ndr) * rss * ods
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, nil
}
