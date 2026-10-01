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

func finitePositive(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && x > 0
}

// VarianceRatio is the overlapping Lo–MacKinlay variance ratio
// VR(q) = Var(r_q) / (q × Var(r_1)) where r_1 are 1-period log returns and
// r_q are overlapping q-period log returns. Mean-reverting series → VR < 1;
// trending / momentum → VR > 1. Returns 1 (neutral) when the ratio is
// undefined (insufficient data, non-finite/non-positive closes, or zero
// 1-period variance).
//
// This measures return autocorrelation, not "channel quality". Smooth
// oscillating channels (e.g. golden tight_range) often have VR ≫ 1.
func VarianceRatio(closes []float64, q int) float64 {
	if q < 1 || len(closes) < q+2 {
		return 1
	}
	r1 := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if !finitePositive(closes[i]) || !finitePositive(closes[i-1]) {
			return 1
		}
		r1 = append(r1, math.Log(closes[i]/closes[i-1]))
	}
	rq := make([]float64, 0, len(closes)-q)
	for i := q; i < len(closes); i++ {
		rq = append(rq, math.Log(closes[i]/closes[i-q]))
	}
	v1 := sampleVariance(r1)
	if v1 <= 0 || math.IsNaN(v1) || math.IsInf(v1, 0) {
		return 1
	}
	vq := sampleVariance(rq)
	if math.IsNaN(vq) || math.IsInf(vq, 0) {
		return 1
	}
	vr := vq / (float64(q) * v1)
	if math.IsNaN(vr) || math.IsInf(vr, 0) {
		return 1
	}
	return vr
}

// MeanReversionScore averages clamp((1−VR(q))×1.5, 0, 1) for q = 4 and q = 8
// (PR-104). Higher when returns mean-revert (negative autocorrelation); zero
// when trending, momentum, or undefined. Not a channel-quality score.
func MeanReversionScore(closes []float64) float64 {
	s4 := clamp01((1 - VarianceRatio(closes, 4)) * 1.5)
	s8 := clamp01((1 - VarianceRatio(closes, 8)) * 1.5)
	out := (s4 + s8) / 2
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return 0
	}
	return out
}

func sampleVariance(xs []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, v := range xs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return math.NaN()
		}
		sum += v
	}
	mean := sum / float64(n)
	var ss float64
	for _, v := range xs {
		d := v - mean
		ss += d * d
	}
	return ss / float64(n-1)
}
