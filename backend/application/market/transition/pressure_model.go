package transition

import "math"

// ExpansionPressure computes the latent pressure for a regime expansion.
//
//	pressure = compressionBreadth * (1 + volatilitySlope) * RegimeAgeFactor(age)
//
// The result is clamped to [0, 1].
func ExpansionPressure(compressionBreadth, volatilitySlope float64, regimeAge int) float64 {
	p := compressionBreadth * (1 + volatilitySlope) * RegimeAgeFactor(regimeAge)
	return math.Min(math.Max(p, 0), 1)
}

// RegimeAgeFactor returns a linear ramp from 0 → 1 over 30 candles.
func RegimeAgeFactor(age int) float64 {
	if age <= 0 {
		return 0
	}
	if age >= 30 {
		return 1
	}
	return float64(age) / 30.0
}

// VolatilitySlope returns the simple rate-of-change between the first and last
// values in a float64 slice.  Falls back to 0 when the input is too short or
// the starting value is zero.
func VolatilitySlope(series []float64) float64 {
	if len(series) < 2 {
		return 0
	}
	start := series[0]
	end := series[len(series)-1]
	if start == 0 {
		return 0
	}
	return (end - start) / start
}
