package setups

import "pano_chart/backend/domain/setup"

// CompressionSetup scores compression-breakout setups.
// Uses compression strength, inverted volatility, and volume.
type CompressionSetup struct{}

// Type returns the setup type identifier.
func (s CompressionSetup) Type() setup.SetupType {
	return setup.CompressionBreakout
}

// Score computes the compression-breakout quality for the given context.
func (s CompressionSetup) Score(ctx SetupContext) float64 {
	compression := ctx.CompressionScore
	volatility := 1 - ctx.Volatility
	volume := ctx.VolumeScore

	score := compression*0.5 + volatility*0.3 + volume*0.2
	return clamp(score)
}
