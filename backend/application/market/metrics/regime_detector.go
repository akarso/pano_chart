package metrics

import (
	"math"

	mkt "pano_chart/backend/domain/market"
)

// detectRegime classifies the current market regime using soft scoring.
// Instead of hard thresholds, it computes a continuous evidence score for
// each regime, applies softmax normalisation, and picks the dominant one.
// Returns the dominant regime, its prevalence (0–1), and the full scores.
func detectRegime(b breadthValues, volatility float64) (mkt.Regime, float64, mkt.RegimeScores) {
	scores := regimeScores(b, volatility)

	// Find dominant regime.
	type entry struct {
		regime mkt.Regime
		score  float64
	}
	candidates := []entry{
		{mkt.RegimeExpansion, scores.Expansion},
		{mkt.RegimeCompression, scores.Compression},
		{mkt.RegimeTrend, scores.Trend},
		{mkt.RegimeSideways, scores.Sideways},
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	return best.regime, best.score, scores
}

// regimeScores computes soft prevalence scores for all four regimes.
// The scores are normalised via softmax so they sum to ~1.0.
//
// All breadth values are on [0, 1], computed from the real domain/scoring
// calculators (the same used by the overview/rankings pipeline):
//   - trend:       mean R² from TrendPredictabilityScoreCalculator
//   - sideways:    mean SidewaysV5 score
//   - compression: mean DetectCompression score
//   - breakout:    mean max(BreakoutUp, BreakoutDown)
//
// volatility is the median short/long ATR ratio (independent metric).
//
// A temperature parameter controls sharpness: higher means the dominant
// regime gets a larger share.
func regimeScores(b breadthValues, volatility float64) mkt.RegimeScores {
	const temperature = 4.0

	// --- Raw evidence for each regime ---

	// Expansion: breakout activity + high volatility.
	expRaw := b.breakout*2 + math.Max(0, volatility-1.0)*3

	// Compression: direct from the real compression detector.
	compRaw := b.compression * 3

	// Trend: direct from the real trend detector.
	trendRaw := b.trend * 3

	// Sideways: direct from the real sideways detector.
	// A small floor (0.1) ensures sideways wins when no other signal is present,
	// mirroring the intuition that "absence of evidence" = sideways.
	sidRaw := b.sideways*2 + 0.1

	// --- Softmax normalisation ---
	raws := [4]float64{
		expRaw * temperature,
		compRaw * temperature,
		trendRaw * temperature,
		sidRaw * temperature,
	}

	maxRaw := raws[0]
	for _, v := range raws[1:] {
		if v > maxRaw {
			maxRaw = v
		}
	}

	var exps [4]float64
	sum := 0.0
	for i, v := range raws {
		exps[i] = math.Exp(v - maxRaw)
		sum += exps[i]
	}

	return mkt.RegimeScores{
		Expansion:   exps[0] / sum,
		Compression: exps[1] / sum,
		Trend:       exps[2] / sum,
		Sideways:    exps[3] / sum,
	}
}
