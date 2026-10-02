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

// SignalWithOutcome joins a signal with an optional resolution.
type SignalWithOutcome struct {
	Signal  Signal
	Outcome *Outcome // nil if unresolved
}

// Filter selects signals for Query (scorecard / debug / export).
type Filter struct {
	Kind      Kind
	Label     string
	Timeframe string
	Symbol    string
	Since     time.Time
	Until     time.Time
	// Limit caps rows. Limit == 0 → default 500 (scorecard). Limit < 0 →
	// unlimited. Positive values are honored as-is.
	Limit int
	// Offset skips the first N rows after ORDER BY (export pagination).
	// Ignored when Limit < 0 (unlimited).
	Offset int
	// OldestFirst orders by emitted_at ASC (default DESC). Use for
	// reproducible training splits.
	OldestFirst bool
}
