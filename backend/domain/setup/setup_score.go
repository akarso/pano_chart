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

	// Confidence inputs. These are read as-is by
	// application/setups.ComputeConfidence — it does not default a zero/
	// unset value to anything else, so a hand-built SetupScores that omits
	// VolatilityFit or SeasonalityFit gets scored as if that reading were
	// the worst possible (0.0), not neutral. Evaluate (application/setups/
	// service.go) is the only caller that populates these correctly for
	// real use, including explicitly setting SeasonalityFit to a neutral
	// 0.5 itself when no seasonality data is available.
	Crowding       float64 // 0–1; position crowding / fragility (high = dangerous)
	VolatilityFit  float64 // 0–1; how suitable current (realized) volatility is for this regime
	SeasonalityFit float64 // 0–1; how favorable the current time-of-day's historical spike-probability is (high = low forward-looking risk) — see PR-082

	// Unified confidence score.
	Confidence float64 // 0–1; contextual validity of the setup

	// Confidence-adjusted breakout probabilities.
	BreakoutUp   float64 // 0–1; upward breakout probability
	BreakoutDown float64 // 0–1; downward breakout probability
}
