package setups

import (
	"math"

	"pano_chart/backend/domain/setup"
)

// VolatilityFit returns a 0–1 score indicating how suitable the current
// (realized, last-N-candle) volatility level is for the given regime.
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

// referenceSpikeProb is the historical spike-probability level treated as
// "maximally risky" (SeasonalityFit reaches its floor of 0 at or above
// this). Unlike VolatilityFit's bell curve (a middling value is ideal),
// low spike probability is unconditionally better than high for a
// forward-looking risk read, so the fit is a straight linear decay, not a
// curve centered on some "ideal" level. This constant is a provisional
// starting point (matching PR-080's own precedent of an honestly-labeled,
// not-yet-calibrated normalization) — revisit once real spike-probability
// distributions have been observed across enough symbols/timeframes to
// pick an evidence-based threshold instead — see PR-082.
const referenceSpikeProb = 0.3

// SeasonalityFit returns a 0–1 score indicating how favorable the current
// time-of-day's historical spike probability is: 1 means this moment has
// historically been calm, 0 means it's historically one of the riskiest
// moments of the day. spikeProb is expected in [0, 1] (a fraction of
// historical days where this specific minute-of-day saw an outlier move —
// see infrastructure/volatility.Aggregate's SpikeProb).
func SeasonalityFit(spikeProb float64) float64 {
	return clamp(1.0 - spikeProb/referenceSpikeProb)
}

// ComputeConfidence returns a unified 0–1 confidence score that answers
// "how much should I trust this setup right now?" using regime-specific
// weighting of trend health, market health, crowding, realized-volatility
// fit, and (PR-082) forward-looking seasonality fit.
//
// Each regime's weight map splits what used to be a single "volatility"
// budget between "volatility" (realized, backward-looking — VolatilityFit)
// and "seasonality" (historical, forward-looking — SeasonalityFit) at a 2:1
// ratio, favoring the realized signal since it's a direct read of actual
// recent price action; seasonality is a coarser prior derived from
// historical time-of-day statistics, and — per PR-082's own scoping
// caveat — currently computed from a single reference symbol (BTCUSDT)
// applied market-wide, not per-symbol. Each regime's total confidence-
// weight distribution across trend/market/crowding/(volatility+seasonality)
// is unchanged from before this split — only the volatility-related slice
// is subdivided.
//
// Contract: every field this reads (TrendHealth, MarketEffective, Crowding,
// VolatilityFit, SeasonalityFit) must already be populated by the caller —
// this function does not apply defaults. Evaluate (service.go) is the
// only correct way to get a well-formed SetupScores for real use; it sets
// SeasonalityFit to a neutral 0.5 itself before this is ever called (when
// no SeasonalityProvider is configured, or the provider errors), so 0.5
// only shows up here as an already-resolved value, never as an implicit
// default. A hand-built SetupScores that omits SeasonalityFit gets Go's
// zero value, 0.0 — read literally as "confirmed maximum seasonality
// risk", not defaulted to neutral (same pre-existing behavior VolatilityFit
// already has for the same reason). Deliberately NOT special-cased here:
// a plain float64 can't distinguish "the caller left this unset" from "the
// provider genuinely reported 0.0 spike-probability fit" — silently
// treating a literal 0.0 as neutral would just as silently understate a
// real worst-case seasonality reading from Evaluate's own successful
// provider call, which would be a worse bug than a hand-built caller's
// score coming out a few hundredths lower than intended.
func ComputeConfidence(s setup.SetupScores) float64 {
	var weights map[string]float64

	switch s.Regime {
	case "uptrend", "downtrend":
		weights = map[string]float64{
			"trend":       0.4,
			"market":      0.3,
			"crowding":    0.2,
			"volatility":  0.07,
			"seasonality": 0.03,
		}
	case "sideways":
		weights = map[string]float64{
			"trend":       0.1,
			"market":      0.3,
			"crowding":    0.3,
			"volatility":  0.2,
			"seasonality": 0.1,
		}
	case "compression":
		weights = map[string]float64{
			"trend":       0.2,
			"market":      0.3,
			"crowding":    0.2,
			"volatility":  0.2,
			"seasonality": 0.1,
		}
	default:
		return 0.5
	}

	score := weights["trend"]*s.TrendHealth +
		weights["market"]*s.MarketEffective +
		weights["crowding"]*(1.0-s.Crowding) +
		weights["volatility"]*s.VolatilityFit +
		weights["seasonality"]*s.SeasonalityFit

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
