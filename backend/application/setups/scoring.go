package setups

import "pano_chart/backend/domain/setup"

// clamp restricts v to the [0, 1] range.
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// TrendHealthModifier returns a nonlinear multiplier for how reliable
// a trend-based setup is given the current health reading.
func TrendHealthModifier(health float64) float64 {
	switch {
	case health > 0.8:
		return 1.05
	case health > 0.6:
		return 1.0
	case health > 0.4:
		return 0.7
	case health > 0.2:
		return 0.4
	default:
		return 0.2
	}
}

// ApplyContextModifier adjusts raw setup scores using trend-health context.
// Only trend-continuation setups are modified; others pass through unchanged.
func ApplyContextModifier(raw setup.SetupScores, ctx SetupContext) setup.SetupScores {
	mod := TrendHealthModifier(ctx.TrendHealth)

	adjusted := make(map[setup.SetupType]float64, len(raw.Scores))
	for k, v := range raw.Scores {
		if k == setup.TrendContinuation {
			adjusted[k] = clamp(v * mod)
		} else {
			adjusted[k] = v
		}
	}

	// Re-pick the best after adjustment.
	bestScore := 0.0
	bestSetup := setup.SetupType("")
	for k, v := range adjusted {
		if v > bestScore {
			bestScore = v
			bestSetup = k
		}
	}

	return setup.SetupScores{
		Symbol:      raw.Symbol,
		Timeframe:   raw.Timeframe,
		BestSetup:   bestSetup,
		Score:       bestScore,
		Scores:      adjusted,
		TrendHealth: ctx.TrendHealth,
		Regime:      ctx.Regime,
	}
}
