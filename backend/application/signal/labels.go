package signal

// SetupLabel maps SetupService BestSetup (+ regime / breakout probs) onto the
// PR-091 outcome vocabulary (compression, range, breakout_up, breakout_down,
// trend_up, trend_down).
func SetupLabel(bestSetup string, regime string, breakoutUp, breakoutDown float64) string {
	switch bestSetup {
	case "compression_breakout":
		if breakoutUp >= 0.5 && breakoutUp >= breakoutDown {
			return "breakout_up"
		}
		if breakoutDown >= 0.5 && breakoutDown > breakoutUp {
			return "breakout_down"
		}
		return "compression"
	case "range_reversion":
		return "range"
	case "trend_continuation":
		switch regime {
		case "downtrend":
			return "trend_down"
		case "uptrend":
			return "trend_up"
		default:
			// Sideways/compression tagged as continuation: use breakout tilt
			// rather than inventing trend_up.
			if breakoutDown > breakoutUp {
				return "trend_down"
			}
			return "trend_up"
		}
	default:
		return bestSetup
	}
}

// BadgeLabel maps DominantComponent + sparkline slope onto PR-091 badge labels.
func BadgeLabel(component string, sparkline []float64) string {
	switch component {
	case "trend":
		if sparklineDown(sparkline) {
			return "trend_down"
		}
		return "trend_up"
	case "sideways":
		return "sideways"
	case "gain":
		return "gain" // no PR-091 rule yet; still logged for future calibration
	default:
		return component
	}
}

func sparklineDown(sp []float64) bool {
	if len(sp) < 2 {
		return false
	}
	return sp[len(sp)-1] < sp[0]
}

// SparklineRange returns min/max of sparkline closes for badge sideways
// Context["range_low"/"range_high"]. Intentional: PR-091 grades "closed
// outside" against closes. Setup emitters use recentExtremes (H/L) for the
// same keys because they have full candles — different window source, same
// grading rule on closes.
func SparklineRange(spark []float64) (lo, hi float64) {
	if len(spark) == 0 {
		return 0, 0
	}
	lo, hi = spark[0], spark[0]
	for _, v := range spark[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}
