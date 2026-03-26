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
}
