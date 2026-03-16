package market

import (
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// classify determines the dominant regime for a single symbol's evaluation.
// Thresholds are intentionally conservative.
func classify(e domain.EvaluationSnapshot) mkt.State {
	if e.BreakoutUpScore > 0.7 || e.BreakoutDownScore > 0.7 {
		return mkt.StateBreakout
	}

	if e.CompressionScore > 0.7 {
		return mkt.StateCompression
	}

	if e.TrendScore > 0.65 {
		return mkt.StateTrend
	}

	return mkt.StateSideways
}
