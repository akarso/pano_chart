package metrics

import mkt "pano_chart/backend/domain/market"

// detectRegime classifies the current market regime from breadth and
// volatility metrics. Returns the detected regime and a confidence score.
// The rules are intentionally simple and interpretable.
func detectRegime(
	trendBreadth float64,
	compressionBreadth float64,
	volatility float64,
) (mkt.Regime, float64) {
	if compressionBreadth > 0.30 && volatility < 0.9 {
		return mkt.RegimeCompression, compressionBreadth
	}

	if trendBreadth > 0.40 && volatility > 1.0 {
		return mkt.RegimeTrend, trendBreadth
	}

	if volatility > 1.3 {
		return mkt.RegimeExpansion, volatility
	}

	return mkt.RegimeSideways, 0.5
}
