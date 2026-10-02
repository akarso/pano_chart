package market

// SectorIndex is a sector composite relative to the market tape (PR-098).
type SectorIndex struct {
	ID          string
	Name        string
	SymbolCount int
	Points      []IndexPoint
	Return      float64 // ln(last/first) on the RS window (clamped to market overlap)
	RS          float64 // sector Return − aligned market Return
	RSAvailable bool    // false when fewer than 2 shared timestamps with market
}

// SectorIndexResult is the payload for GET /api/market/sectors.
type SectorIndexResult struct {
	Timeframe         string
	MarketSymbolCount int // contributors to the market baseline (post activePaths)
	Sectors           []SectorIndex
}
