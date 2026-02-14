package ports

import (
	"time"

	"pano_chart/backend/domain"
)

// CandleRepositoryPort is a read-only interface for retrieving candle data.
// It operates strictly on domain objects.
// Implementations are responsible for persisting and retrieving candles from infrastructure.
type CandleRepositoryPort interface {
	// GetSeries retrieves a CandleSeries for a given symbol and timeframe within a time range.
	//
	// Parameters:
	//   - symbol: the tradable instrument
	//   - timeframe: the candle aggregation interval
	//   - from: inclusive start time (UTC)
	//   - to: exclusive end time (UTC)
	//
	// Returns:
	//   - A CandleSeries ordered by timestamp (may be empty)
	//   - An error if retrieval fails or arguments are invalid
	//
	// The returned series may contain gaps; this is expected when data is not available.
	GetSeries(
		symbol domain.Symbol,
		timeframe domain.Timeframe,
		from time.Time,
		to time.Time,
	) (domain.CandleSeries, error)

	// GetLastNCandles retrieves the last N completed candles for a given symbol and timeframe.
	//
	// Parameters:
	//   - symbol: the tradable instrument
	//   - timeframe: the candle aggregation interval
	//   - n: number of recent completed candles to retrieve
	//
	// Returns:
	//   - A CandleSeries ordered by timestamp (oldest to newest)
	//   - If fewer than N candles exist, returns all available candles
	//   - An error if retrieval fails or arguments are invalid
	//
	// Implementation notes:
	//   - Must exclude in-progress (incomplete) candles
	//   - Must return exactly N completed candles if available
	//   - Must handle candle alignment internally (no time math in caller)
	GetLastNCandles(
		symbol domain.Symbol,
		timeframe domain.Timeframe,
		n int,
	) (domain.CandleSeries, error)
}
