package setups

import (
	"math"

	"pano_chart/backend/domain/setup"
)

// VolatilityFit returns a 0–1 score indicating how suitable the current
// volatility level is for the given regime.
func VolatilityFit(regime string, volatility float64) float64 {
	switch regime {
	case "uptrend", "downtrend":
		// Moderate volatility is ideal for trend continuation.
		return clamp(1.0 - 2.0*math.Abs(volatility-0.35))
	case "sideways":
		// Moderate volatility suits range reversion.
		return clamp(1.0 - 2.0*math.Abs(volatility-0.5))
	case "compression":
		// Low volatility is ideal for compression (tight squeeze).
		return clamp(1.0 - volatility)
	default:
		return 0.5
	}
}

// ComputeConfidence returns a unified 0–1 confidence score that answers
// "how much should I trust this setup right now?" using regime-specific
// weighting of trend health, market health, crowding, and volatility fit.
func ComputeConfidence(s setup.SetupScores) float64 {
	var weights map[string]float64

	switch s.Regime {
	case "uptrend", "downtrend":
		weights = map[string]float64{
			"trend":      0.4,
			"market":     0.3,
			"crowding":   0.2,
			"volatility": 0.1,
		}
	case "sideways":
		weights = map[string]float64{
			"trend":      0.1,
			"market":     0.3,
			"crowding":   0.3,
			"volatility": 0.3,
		}
	case "compression":
		weights = map[string]float64{
			"trend":      0.2,
			"market":     0.3,
			"crowding":   0.2,
			"volatility": 0.3,
		}
	default:
		return 0.5
	}

	score := weights["trend"]*s.TrendHealth +
		weights["market"]*s.MarketEffective +
		weights["crowding"]*(1.0-s.Crowding) +
		weights["volatility"]*s.VolatilityFit

	return clamp(score)
}

// ConfidenceLabel returns a human-readable label for a confidence value.
func ConfidenceLabel(confidence float64) string {
	switch {
	case confidence > 0.75:
		return "High"
	case confidence > 0.55:
		return "Medium"
	default:
		return "Low"
	}
}
