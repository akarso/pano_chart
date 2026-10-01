package setups

// SetupContext is the market-data snapshot passed to every SetupEvaluator.
// The values are pre-computed from candle data and existing scoring algorithms.
type SetupContext struct {
	Symbol string

	CompressionScore float64
	TrendScore       float64
	RangeScore       float64

	VolumeScore    float64
	LiquidityScore float64

	Volatility float64

	TrendHealth float64 // 0–1 health of the underlying trend
	Regime      string  // dominant regime label

	// MeanReversionScore is Lo–MacKinlay MRS on the Sideways V5 window
	// (last CandleCount closes, PR-104). 0 = no mean reversion (also the Go
	// zero value — fail-closed for PR-110 RangeQuality). This is return
	// autocorrelation, not channel quality: smooth ranges like tight_range
	// typically score ~0. Not filled by buildContext until a consumer needs
	// it; use meanReversionFromSeries to populate.
	MeanReversionScore float64
}
