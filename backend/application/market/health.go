package market

import mkt "pano_chart/backend/domain/market"

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

// DampenTrendByHealth reduces trend breadth when trend health is poor and
// redistributes the lost weight proportionally among the other regimes so
// the four breadth values still sum to ~1.0.
//
// The dampening factor combines two signals:
//   - effectiveTrend: average health of trending tokens (0–1)
//   - breakdownRate:  fraction of trending tokens breaking down (0–1)
//
// A healthy market (effectiveTrend=0.8, breakdownRate=0.1) barely dampens.
// A breaking market (effectiveTrend=0.2, breakdownRate=0.8) dramatically
// reduces the trend prevalence, allowing other regimes to surface.
func DampenTrendByHealth(b mkt.Breadth, effectiveTrend, breakdownRate float64) mkt.Breadth {
	// Health factor: high effective trend → factor near 1.0.
	// Breakdown penalty: high breakdown rate → extra reduction.
	healthFactor := clamp(effectiveTrend*1.5, 0, 1) // 0.67+ health → full credit
	breakdownPenalty := breakdownRate * 0.5         // up to 50% penalty from breakdowns
	dampFactor := clamp(healthFactor-breakdownPenalty, 0.1, 1.0)

	lost := b.Trend * (1.0 - dampFactor)
	b.Trend *= dampFactor

	// Redistribute lost weight proportionally to other regimes.
	otherSum := b.Sideways + b.Compression + b.Expansion
	if otherSum > 0 && lost > 0 {
		b.Sideways += lost * (b.Sideways / otherSum)
		b.Compression += lost * (b.Compression / otherSum)
		b.Expansion += lost * (b.Expansion / otherSum)
	} else if lost > 0 {
		// All other regimes are zero — assign to sideways as default.
		b.Sideways += lost
	}

	return b
}
