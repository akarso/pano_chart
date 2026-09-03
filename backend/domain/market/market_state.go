package market

// State represents the dominant market regime across the symbol universe.
type State string

const (
	StateSideways    State = "sideways"
	StateCompression State = "compression"
	StateExpansion   State = "expansion"
	StateTrend       State = "trend"
	StateSilent      State = "silent"
	StateIndecisive  State = "indecisive"
)

// Breadth holds the proportionally-weighted breadth for each regime.
// Each symbol distributes its scores continuously across the four
// buckets, so the values reflect the true market character.
// All values are in [0, 1] and sum to ~1.
type Breadth struct {
	Sideways    float64
	Compression float64
	Expansion   float64
	Trend       float64
}

// Summary is the aggregate market state for a given timeframe.
type Summary struct {
	Timeframe   string
	State       State
	Confidence  float64
	Breadth     Breadth
	SymbolCount int
	// Bias indicates the dominant direction when State is trend:
	// "up", "down", or "neutral".
	Bias string

	// Trend health aggregates (additive — zero values are backward compatible).
	EffectiveTrend float64 // average trend health across all tokens (0–1)
	BreakdownRate  float64 // fraction of trending tokens with health < 0.4
	Label          string  // human-readable quality label
}
