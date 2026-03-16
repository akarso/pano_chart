package market

// IndexPoint represents a single data point in the composite index time series.
type IndexPoint struct {
	Timestamp int64
	Value     float64
}

// CompositeIndex is a normalized market index derived from all scanned symbols.
// Values are rebased to 100 at the first candle, so 101 ≈ market +1%.
type CompositeIndex struct {
	Timeframe   string
	Points      []IndexPoint
	SymbolCount int
}
