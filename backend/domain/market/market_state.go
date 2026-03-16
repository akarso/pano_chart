package market

// State represents the dominant market regime across the symbol universe.
type State string

const (
	StateSideways    State = "sideways"
	StateCompression State = "compression"
	StateBreakout    State = "breakout"
	StateTrend       State = "trend"
)

// Breadth holds the fraction of symbols classified into each regime.
// All values are in [0, 1] and sum to 1.
type Breadth struct {
	Sideways    float64
	Compression float64
	Breakout    float64
	Trend       float64
}

// Summary is the aggregate market state for a given timeframe.
type Summary struct {
	Timeframe   string
	State       State
	Confidence  float64
	Breadth     Breadth
	SymbolCount int
}
