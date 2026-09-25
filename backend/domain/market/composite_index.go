package market

// IndexPoint represents a single data point in the composite index time series.
type IndexPoint struct {
	Timestamp int64
	Value     float64
}

// CompositeIndex is a normalized market index derived from scanned symbols.
// Values start at 100; subsequent points compound aggregated log-returns
// (PR-095), so 101 ≈ +1% from the prior bar on the chosen path.
//
// Points is the equal-weight median of log-returns (outlier-resistant).
// VolumeWeightedPoints is the quote-volume-weighted mean of log-returns.
// Either slice may be empty when insufficient data exists for that variant.
type CompositeIndex struct {
	Timeframe            string
	Points               []IndexPoint // median
	VolumeWeightedPoints []IndexPoint // volume-weighted mean
	SymbolCount          int
}
