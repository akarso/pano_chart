package market

// ComputeTrendHealth returns a 0–1 score indicating how "healthy" a trend
// is for the given token. A score near 1 means the trend is intact; near 0
// means it is breaking down.
//
// state must be "uptrend" or "downtrend"; all other values return 0.
// atr == 0 returns 0 (no volatility baseline).
func ComputeTrendHealth(state string, price, recentHigh, recentLow, atr, recentReturn float64) float64 {
	if atr == 0 {
		return 0
	}

	var health float64

	switch state {
	case "uptrend":
		drawdown := (recentHigh - price) / atr
		health = 1.0 - clamp(drawdown, 0, 1)
	case "downtrend":
		bounce := (price - recentLow) / atr
		health = 1.0 - clamp(bounce, 0, 1)
	default:
		return 0
	}

	// Crash penalty: a large adverse move suggests the trend is breaking.
	if recentReturn < -1.5 {
		health *= 0.3
	}

	return clamp(health, 0, 1)
}

// BuildMarketLabel produces a human-readable label based on aggregate
// trend prevalence and effective trend health.
func BuildMarketLabel(trendPrevalence, effectiveTrend float64) string {
	if trendPrevalence > 0.6 {
		if effectiveTrend > 0.5 {
			return "Strong trend"
		}
		if effectiveTrend > 0.3 {
			return "Trend weakening"
		}
		return "Trend breaking down"
	}

	if trendPrevalence > 0.4 {
		return "Mixed conditions"
	}

	return "No clear trend"
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
