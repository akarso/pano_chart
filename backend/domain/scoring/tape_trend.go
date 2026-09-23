package scoring

import "math"

// TapeTrend scores directional persistence on a close series without the
// shape / clustering gates used by Trend Predictability (PR-115).
// atr14 is Wilder true ATR; pass 0 when unknown (Mag becomes 0).
// bias is "up", "down", or "neutral".
func TapeTrend(closes []float64, atr14 float64) (score float64, bias string) {
	bias = "neutral"
	n := len(closes)
	if n < 2 {
		return 0, bias
	}
	net := closes[n-1] - closes[0]
	var path float64
	for i := 1; i < n; i++ {
		path += math.Abs(closes[i] - closes[i-1])
	}
	er := 0.0
	if path > 0 {
		er = math.Abs(net) / path
	}
	_, r2, ok := OLSSlopeR2(closes)
	if !ok {
		r2 = 0
	}
	mag := 0.0
	if atr14 > 0 {
		mag = clamp01(math.Abs(net) / (4 * atr14))
	}
	score = r2 * mag * clamp01(er/0.25)
	if score > 1 {
		score = 1
	}
	if score >= 0.05 {
		switch {
		case net > 0:
			bias = "up"
		case net < 0:
			bias = "down"
		}
	}
	return score, bias
}
