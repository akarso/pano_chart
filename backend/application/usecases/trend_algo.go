package usecases

import (
	"pano_chart/backend/domain/scoring"
)

// TrendAlgoMode selects the trend score calculator (PR-103).
type TrendAlgoMode string

const (
	TrendAlgoPredictability TrendAlgoMode = "predictability"
	TrendAlgoStrength       TrendAlgoMode = "strength"
)

// ParseTrendAlgo normalises s to a TrendAlgoMode.
// ok is false for unrecognized non-empty values (caller should warn and
// treat the returned predictability fallback as a misconfiguration).
// Empty string is a valid default → predictability, ok=true.
func ParseTrendAlgo(s string) (TrendAlgoMode, bool) {
	switch TrendAlgoMode(s) {
	case "", TrendAlgoPredictability:
		return TrendAlgoPredictability, true
	case TrendAlgoStrength:
		return TrendAlgoStrength, true
	default:
		return TrendAlgoPredictability, false
	}
}

// TrendCalcFor returns the directed trend calculator for rankings / setups.
// Both engines still report Name() `"Trend Predictability"` for weight-key
// stability; use TrendAlgoMode (config/env/logs) to know which ran.
func TrendCalcFor(algo TrendAlgoMode) scoring.DirectedScoreCalculator {
	if algo == TrendAlgoStrength {
		return &scoring.TrendStrengthScoreCalculator{}
	}
	return &scoring.TrendPredictabilityScoreCalculator{}
}
