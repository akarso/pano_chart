package symbol_universe

import (
	"context"
	"pano_chart/backend/domain"
)

// SymbolUniverseProvider defines the contract for symbol universe providers.
type SymbolUniverseProvider interface {
	Symbols(ctx context.Context, exchangeInfoURL, tickerURL string) ([]domain.Symbol, error)
}
