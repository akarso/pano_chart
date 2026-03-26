package setup

// SetupType identifies a specific trade setup strategy.
type SetupType string

const (
	CompressionBreakout SetupType = "compression_breakout"
	TrendContinuation   SetupType = "trend_continuation"
	RangeReversion      SetupType = "range_reversion"
)

// SetupScores is the domain result of evaluating all setup strategies
// for a single symbol on a given timeframe.
type SetupScores struct {
	Symbol    string
	Timeframe string
	BestSetup SetupType
	Score     float64
	Scores    map[SetupType]float64

	// Trend health context (only meaningful when BestSetup is trend-based).
	TrendHealth float64 // 0–1; health of the underlying trend
	Regime      string  // dominant regime: "uptrend", "downtrend", "sideways", "compression"

	// Market-level context.
	MarketEffective float64 // 0–1; aggregate market trend strength

	// Confidence inputs.
	Crowding      float64 // 0–1; position crowding / fragility (high = dangerous)
	VolatilityFit float64 // 0–1; how suitable current volatility is for this regime

	// Unified confidence score.
	Confidence float64 // 0–1; contextual validity of the setup

	// Confidence-adjusted breakout probabilities.
	BreakoutUp   float64 // 0–1; upward breakout probability
	BreakoutDown float64 // 0–1; downward breakout probability
}
