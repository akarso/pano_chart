package market

// Regime represents the detected market regime based on aggregated metrics.
type Regime string

const (
	RegimeCompression Regime = "compression"
	RegimeSideways    Regime = "sideways"
	RegimeTrend       Regime = "trend"
	RegimeExpansion   Regime = "expansion"
)

// RegimeMetrics holds the computed market-level metrics used to detect the regime.
type RegimeMetrics struct {
	TrendBreadth        float64
	CompressionBreadth  float64
	VolatilityExpansion float64
	Dispersion          float64
}

// RegimeSummary is the full regime detection result for a given timeframe.
type RegimeSummary struct {
	Timeframe  string
	Regime     Regime
	Confidence float64
	Metrics    RegimeMetrics
}
