package market

import (
	"math"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// scoreWeights returns a proportional distribution across regimes for a single
// symbol. The raw scores are normalised so they sum to 1, giving each token a
// continuous "vote" rather than a binary bucket.
func scoreWeights(e domain.EvaluationSnapshot) mkt.Breadth {
	breakout := math.Max(e.BreakoutUpScore, e.BreakoutDownScore)
	compression := e.CompressionScore
	trend := e.TrendScore
	sideways := e.SidewaysScore

	total := breakout + compression + trend + sideways
	if total == 0 {
		return mkt.Breadth{Sideways: 1}
	}

	return mkt.Breadth{
		Sideways:    sideways / total,
		Compression: compression / total,
		Breakout:    breakout / total,
		Trend:       trend / total,
	}
}
