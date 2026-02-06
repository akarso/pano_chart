
package scoring

import (
	"pano_chart/backend/domain"
	"fmt"
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
	return slopeNorm * R2, nil
}
