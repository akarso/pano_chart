package setups

import "pano_chart/backend/domain/setup"

// MarketModifier returns the market-level multiplier for a given setup score
// and market effective trend strength.
func MarketModifier(scores setup.SetupScores, effective float64) float64 {
	switch scores.Regime {
	case "uptrend", "downtrend":
		return trendMarketModifier(effective)
	case "sideways":
		return sidewaysMarketModifier(effective)
	case "compression":
		return compressionMarketModifier(effective)
	default:
		return 1.0
	}
}

// trendMarketModifier boosts trend setups in strong markets, punishes in weak.
func trendMarketModifier(effective float64) float64 {
	switch {
	case effective > 0.6:
		return 1.1
	case effective > 0.4:
		return 1.0
	case effective > 0.25:
		return 0.7
	default:
		return 0.4
	}
}

// sidewaysMarketModifier rewards range-based setups in weak/chaotic markets.
func sidewaysMarketModifier(effective float64) float64 {
	switch {
	case effective < 0.3:
		return 1.1
	case effective < 0.5:
		return 1.0
	default:
		return 0.8
	}
}

// compressionMarketModifier favours compression in transition zones.
func compressionMarketModifier(effective float64) float64 {
	switch {
	case effective > 0.4 && effective < 0.6:
		return 1.1
	case effective < 0.3:
		return 0.9
	case effective > 0.7:
		return 0.9
	default:
		return 1.0
	}
}

// ApplyMarketModifier applies the global market modifier to an already
// locally-adjusted SetupScores. It scales ALL sub-scores by their regime-
// appropriate multiplier and re-picks the best.
func ApplyMarketModifier(scores setup.SetupScores, effective float64) setup.SetupScores {
	mod := MarketModifier(scores, effective)

	adjusted := make(map[setup.SetupType]float64, len(scores.Scores))
	for k, v := range scores.Scores {
		adjusted[k] = clamp(v * mod)
	}

	bestScore := 0.0
	bestSetup := setup.SetupType("")
	for k, v := range adjusted {
		if v > bestScore {
			bestScore = v
			bestSetup = k
		}
	}

	scores.Scores = adjusted
	scores.BestSetup = bestSetup
	scores.Score = bestScore
	scores.MarketEffective = effective
	return scores
}

// MarketLabel returns a human-readable label for the market effective strength.
func MarketLabel(effective float64) string {
	switch {
	case effective > 0.6:
		return "Favorable"
	case effective >= 0.4:
		return "Neutral"
	default:
		return "Unfavorable"
	}
}
