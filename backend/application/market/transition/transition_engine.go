package transition

import mkt "pano_chart/backend/domain/market"

// TransitionEngine computes regime-transition probabilities from the current
// regime and a set of market-level signals.  It is a pure, stateless calculator.
type TransitionEngine struct{}

// NewTransitionEngine constructs the engine.
func NewTransitionEngine() *TransitionEngine {
	return &TransitionEngine{}
}

// Calculate returns the probability of transitioning from the current regime
// to each other regime.
//
// Parameters:
//   - regime:             current detected regime
//   - compressionBreadth: fraction of symbols in compression [0,1]
//   - volSlope:           rate-of-change of volatility expansion
//   - regimeAge:          how many candles the current regime has persisted
func (e *TransitionEngine) Calculate(
	regime mkt.Regime,
	compressionBreadth float64,
	volSlope float64,
	regimeAge int,
) mkt.TransitionProbabilities {
	switch regime {
	case mkt.RegimeCompression:
		return e.fromCompression(compressionBreadth, volSlope, regimeAge)
	case mkt.RegimeTrend:
		return e.fromTrend(compressionBreadth, volSlope, regimeAge)
	case mkt.RegimeExpansion:
		return e.fromExpansion()
	default: // sideways
		return e.fromSideways(compressionBreadth)
	}
}

// fromCompression uses breakout-pressure to split probability between trend
// and expansion.  When pressure is low the market is likely to stay in
// compression (mapped into the sideways bucket here).
func (e *TransitionEngine) fromCompression(compBreadth, volSlope float64, age int) mkt.TransitionProbabilities {
	pressure := BreakoutPressure(compBreadth, volSlope, age)

	// pressure → breakout probability; the complement stays sideways-ish.
	trendP := pressure * 0.6
	expansionP := pressure * 0.4
	sidewaysP := 1 - trendP - expansionP

	return mkt.TransitionProbabilities{
		Trend:     trendP,
		Sideways:  sidewaysP,
		Expansion: expansionP,
	}
}

// fromTrend: strong trends tend to persist; breakout pressure can signal
// reversal toward expansion.
func (e *TransitionEngine) fromTrend(compBreadth, volSlope float64, age int) mkt.TransitionProbabilities {
	pressure := BreakoutPressure(compBreadth, volSlope, age)

	// continuation probability decreases with pressure.
	trendP := 0.6 * (1 - pressure)
	expansionP := 0.2 + 0.3*pressure
	sidewaysP := 1 - trendP - expansionP

	return mkt.TransitionProbabilities{
		Trend:     trendP,
		Sideways:  sidewaysP,
		Expansion: expansionP,
	}
}

// fromExpansion: expansion is mean-reverting → high probability of returning
// to sideways or compression (represented by sideways here).
func (e *TransitionEngine) fromExpansion() mkt.TransitionProbabilities {
	return mkt.TransitionProbabilities{
		Trend:     0.25,
		Sideways:  0.55,
		Expansion: 0.20,
	}
}

// fromSideways: sideways drifts toward breakout targets as compressionBreadth
// rises.  The compression pressure is split 60/40 between trend and expansion.
func (e *TransitionEngine) fromSideways(compBreadth float64) mkt.TransitionProbabilities {
	pressureShift := compBreadth * 0.5
	trendP := 0.2 + pressureShift*0.6
	expansionP := 0.1 + pressureShift*0.4
	sidewaysP := 1 - trendP - expansionP

	// guard against negative values when compBreadth is very high
	if sidewaysP < 0 {
		sidewaysP = 0
	}

	return mkt.TransitionProbabilities{
		Trend:     trendP,
		Sideways:  sidewaysP,
		Expansion: expansionP,
	}
}
