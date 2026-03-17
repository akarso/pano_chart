package setups

import "pano_chart/backend/domain/setup"

// RangeSetup scores range-reversion setups.
// Uses range score and volatility.
type RangeSetup struct{}

// Type returns the setup type identifier.
func (s RangeSetup) Type() setup.SetupType {
	return setup.RangeReversion
}

// Score computes the range-reversion quality for the given context.
func (s RangeSetup) Score(ctx SetupContext) float64 {
	rangeScore := ctx.RangeScore
	volatility := ctx.Volatility

	score := rangeScore*0.7 + volatility*0.3
	return clamp(score)
}
