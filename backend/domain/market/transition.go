package market

// TransitionProbabilities holds the probability of transitioning to each regime.
// Values are in [0, 1] and should sum to ≈1.0 (after the current regime is excluded).
type TransitionProbabilities struct {
	Trend       float64
	Sideways    float64
	Compression float64
	Expansion   float64
}

// MarketTransition is the full transition-probability result for a timeframe.
type MarketTransition struct {
	Timeframe     string
	CurrentRegime Regime
	Probabilities TransitionProbabilities
	Horizon       string // e.g. "12 candles"

	// Source is "heuristic" when no empirical samples apply, else "blend" (PR-107).
	Source string
	// EmpiricalWeight is the blend weight w ∈ [0, 0.7] applied to history.
	EmpiricalWeight float64
	// SampleSize is the raw transition count for the row actually used
	// (age bucket or pooled all-age).
	SampleSize int
	// Pooled is true when sampleSize / probs came from the all-age row
	// rather than the age bucket for the current regime age.
	Pooled bool
}
