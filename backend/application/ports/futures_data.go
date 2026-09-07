package ports

import "context"

// FuturesDataPort provides real Binance Futures market data used for
// fragility/crowding scoring — funding rate, open interest history, and the
// global long/short account ratio. Implementations should return a clear
// error for a symbol with no futures market, rather than zero values, so
// callers can distinguish "no data" from "genuinely neutral" — see PR-081.
type FuturesDataPort interface {
	// FundingRate returns the current funding rate for symbol as a signed
	// fraction (e.g. 0.0001 = 0.01%).
	FundingRate(ctx context.Context, symbol string) (float64, error)

	// OpenInterestHistory returns recent open-interest values for symbol,
	// ordered oldest to newest.
	OpenInterestHistory(ctx context.Context, symbol string) ([]float64, error)

	// LongShortRatio returns the global long-account ratio for symbol, in
	// [0, 1] (0.5 = neutral, evenly split between long and short accounts).
	LongShortRatio(ctx context.Context, symbol string) (float64, error)
}
