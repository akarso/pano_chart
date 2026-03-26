package setups

import "pano_chart/backend/domain/setup"

// ComputeBreakoutProbability adjusts a raw breakout score by confidence.
// The formula keeps the base meaningful but scales down unreliable signals:
//
//	adjustment = 0.5 + 0.5 × confidence
//	probability = baseScore × adjustment
//
// Confidence 1.0 → multiplier 1.0 (no change)
// Confidence 0.5 → multiplier 0.75
// Confidence 0.0 → multiplier 0.5
func ComputeBreakoutProbability(baseScore, confidence float64) float64 {
	adjustment := 0.5 + 0.5*confidence
	prob := baseScore * adjustment

	// Low confidence floor: avoid misleading spikes.
	if confidence < 0.3 {
		prob *= 0.8
	}

	return clamp(prob)
}

// ApplyBreakoutConfidence computes confidence-adjusted breakout probabilities
// and applies an optional directional bias fix for trend health.
func ApplyBreakoutConfidence(scores setup.SetupScores, rawUp, rawDown float64) setup.SetupScores {
	up := rawUp
	down := rawDown

	// Directional bias fix: penalise up-breakout when trend health is weak.
	if scores.TrendHealth < 0.4 {
		up *= 0.7
	}

	scores.BreakoutUp = ComputeBreakoutProbability(up, scores.Confidence)
	scores.BreakoutDown = ComputeBreakoutProbability(down, scores.Confidence)
	return scores
}
