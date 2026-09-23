package scoring

import (
	"math"

	"pano_chart/backend/domain"
)

// OLSSlopeR2 fits y = a + b·x over indexed values (x = 0..n-1) and returns
// the slope and R². ok is false when there are fewer than two points or the
// series has zero variance (flat).
func OLSSlopeR2(y []float64) (slope, r2 float64, ok bool) {
	n := len(y)
	if n < 2 {
		return 0, 0, false
	}
	var sumX, sumY float64
	for i, v := range y {
		sumX += float64(i)
		sumY += v
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	var num, den, ssTot float64
	for i, v := range y {
		dx := float64(i) - meanX
		dy := v - meanY
		num += dx * dy
		den += dx * dx
		ssTot += dy * dy
	}
	if den == 0 || ssTot == 0 {
		return 0, 0, false
	}
	slope = num / den
	var ssRes float64
	for i, v := range y {
		fit := meanY + slope*(float64(i)-meanX)
		d := v - fit
		ssRes += d * d
	}
	return slope, 1 - ssRes/ssTot, true
}

// TrueATR returns Wilder's true average true range over candles.
// period must be ≥ 1. Returns 0 when there are not enough bars.
func TrueATR(candles []domain.Candle, period int) float64 {
	if period < 1 || len(candles) < period+1 {
		return 0
	}
	trs := make([]float64, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		r1 := candles[i].High() - candles[i].Low()
		r2 := math.Abs(candles[i].High() - candles[i-1].Close())
		r3 := math.Abs(candles[i].Low() - candles[i-1].Close())
		trs[i-1] = math.Max(r1, math.Max(r2, r3))
	}
	if len(trs) < period {
		return 0
	}
	var atr float64
	for i := 0; i < period; i++ {
		atr += trs[i]
	}
	atr /= float64(period)
	for i := period; i < len(trs); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}
	return atr
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
