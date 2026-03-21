package market

// Regime represents the detected market regime based on aggregated metrics.
type Regime string

const (
	RegimeCompression Regime = "compression"
	RegimeSideways    Regime = "sideways"
	RegimeTrend       Regime = "trend"
	RegimeExpansion   Regime = "expansion"
)

// RegimeScores holds the soft prevalence score for each regime.
// All four values sum to ~1.0.
type RegimeScores struct {
	Expansion   float64
	Compression float64
	Trend       float64
	Sideways    float64
}

// RegimeMetrics holds the computed market-level metrics used to detect the regime.
type RegimeMetrics struct {
	TrendBreadth        float64
	SidewaysBreadth     float64
	CompressionBreadth  float64
	BreakoutBreadth     float64
	VolatilityExpansion float64
	Dispersion          float64
}

// RegimeSummary is the full regime detection result for a given timeframe.
type RegimeSummary struct {
	Timeframe  string
	Regime     Regime       // dominant regime (highest score)
	Prevalence float64      // dominant regime's score (0–1)
	Scores     RegimeScores // all regime scores (sum to ~1.0)
	Metrics    RegimeMetrics
}
