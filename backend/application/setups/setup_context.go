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
}
