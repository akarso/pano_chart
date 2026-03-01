package scoring

import (
	"fmt"
	"math"
	"pano_chart/backend/domain"
)

// SidewaysV4ScoreCalculator builds on v1 with additional drift penalty.
type SidewaysV4ScoreCalculator struct{}

func (c *SidewaysV4ScoreCalculator) Name() string {
	return "Sideways Consistency"
}

func (c *SidewaysV4ScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	n := series.Len()
	if n < 6 {
		return 0, fmt.Errorf("at least 6 candles required")
	}
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		candle, _ := series.At(i)
		closes[i] = candle.Close()
	}
	// --- v1 score ---
	v1 := &SidewaysConsistencyScoreCalculator{}
	base, err := v1.Score(series)
	if err != nil {
		return 0, err
	}
	// --- Drift penalty: penalize net displacement more strongly ---
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
	drift := math.Abs(pn-p0) / (rangePrice + 1e-9)
	driftPenalty := 1.0 - drift*drift // quadratic penalty
	if driftPenalty < 0 {
		driftPenalty = 0
	}
	score := base * driftPenalty
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, nil
}
