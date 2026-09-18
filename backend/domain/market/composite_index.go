package market

// IndexPoint represents a single data point in the composite index time series.
type IndexPoint struct {
	Timestamp int64
	Value     float64
}

// CompositeIndex is a normalized market index derived from all scanned symbols.
// Values are rebased to 100 at the first candle, so 101 ≈ market +1%.
//
// Points is the equal-weight median path (outlier-resistant).
// VolumeWeightedPoints is the quote-volume-weighted mean path (money-flow weighted).
// Either slice may be empty when insufficient data exists for that variant.
type CompositeIndex struct {
	Timeframe            string
	Points               []IndexPoint // median
	VolumeWeightedPoints []IndexPoint // volume-weighted mean
	SymbolCount          int
}
