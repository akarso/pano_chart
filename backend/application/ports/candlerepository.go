package ports

import (
	"time"

	"github.com/akarso/pano_chart/backend/domain"
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
}
