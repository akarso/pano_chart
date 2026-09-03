package market

// RegimePeriod represents a contiguous block of time during which the market
// remained in a single regime.
type RegimePeriod struct {
	Regime          Regime
	StartTimestamp  int64
	EndTimestamp    *int64 // nil when the period is still active
	DurationCandles int
}

// RegimeHistory is the full history of regime periods for a timeframe.
type RegimeHistory struct {
	Timeframe  string
	Periods    []RegimePeriod
	CurrentAge int // duration in candles of the most recent period
}
