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
}
