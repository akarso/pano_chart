package setups

import "pano_chart/backend/domain/setup"

// TrendSetup scores trend-continuation setups.
// Uses trend strength, volume support, and low counter-volatility.
type TrendSetup struct{}

// Type returns the setup type identifier.
func (s TrendSetup) Type() setup.SetupType {
	return setup.TrendContinuation
}

// Score computes the trend-continuation quality for the given context.
func (s TrendSetup) Score(ctx SetupContext) float64 {
	trend := ctx.TrendScore
	volume := ctx.VolumeScore
	stability := 1 - ctx.Volatility

	score := trend*0.6 + volume*0.3 + stability*0.1
	return clamp(score)
}
