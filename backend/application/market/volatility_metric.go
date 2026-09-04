package market

import (
	"math"

	"pano_chart/backend/domain"
)

const (
	longATRPeriod  = 30 // candles for the long-term ATR baseline
	shortATRPeriod = 7  // candles for the short-term ATR
)

// volatilityExpansion computes the ratio of short-term ATR to long-term ATR.
// A ratio > 1.3 indicates expansion, ~1 is normal, < 0.8 is compression.
func volatilityExpansion(candles []domain.Candle) float64 {
	if len(candles) < longATRPeriod {
		return 1
	}

	shortATR := atr(candles[len(candles)-shortATRPeriod:])
	longATR := atr(candles[len(candles)-longATRPeriod:])

	if longATR == 0 {
		return 1
	}

	return shortATR / longATR
}

// atr computes the Average True Range over a slice of candles.
func atr(candles []domain.Candle) float64 {
	if len(candles) < 2 {
		return 0
	}

	sum := 0.0
	for i := 1; i < len(candles); i++ {
		prev := candles[i-1]
		curr := candles[i]

		highLow := curr.High() - curr.Low()
		highClose := math.Abs(curr.High() - prev.Close())
		lowClose := math.Abs(curr.Low() - prev.Close())

		tr := math.Max(highLow, math.Max(highClose, lowClose))
		sum += tr
	}

	return sum / float64(len(candles)-1)
}
