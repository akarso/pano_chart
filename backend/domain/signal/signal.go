package signal

import "time"

// Kind identifies the user-facing call that produced a signal.
type Kind string

const (
	KindBadge      Kind = "badge"      // rankings ↑T / ↔S / gain badge
	KindSetup      Kind = "setup"      // SetupService result with Confidence ≥ 0.5
	KindRegime     Kind = "regime"     // tape regime change (observer)
	KindTransition Kind = "transition" // transition prob for a target regime ≥ 0.5
)

// DefaultHorizonBars is how many bars until evaluation when unset.
const DefaultHorizonBars = 20

// Signal is a user-facing call recorded for later grading (PR-090).
type Signal struct {
	ID          string
	Kind        Kind
	Symbol      string // "" for market-wide
	Timeframe   string
	Label       string // e.g. "trend", "sideways", "regime:trend", "transition:sideways"
	Score       float64
	Price       float64
	ATR         float64
	Context     map[string]float64
	EmittedAt   time.Time
	HorizonBars int
}

// Outcome is filled by the PR-091 evaluator. Defined here so MarkResolved
// can accept it without a circular dependency.
type Outcome struct {
	SignalID      string
	ResolvedAt    time.Time
	ForwardReturn float64
	MaxFavorable  float64
	MaxAdverse    float64
	Success       bool
	Rule          string
}

// SignalWithOutcome joins a signal with an optional resolution.
type SignalWithOutcome struct {
	Signal  Signal
	Outcome *Outcome // nil if unresolved
}

// Filter selects signals for Query (scorecard / debug).
type Filter struct {
	Kind      Kind
	Label     string
	Timeframe string
	Symbol    string
	Since     time.Time
	Until     time.Time
	Limit     int
}
