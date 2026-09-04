package market

import (
	"context"

	"pano_chart/backend/application/ports"
	suPorts "pano_chart/backend/application/symbol_universe"
	"pano_chart/backend/domain"
)

// CompositeCandleProvider adapts the existing CandleRepositoryPort and
// SymbolUniverseProvider to the metrics.CandleProvider interface.
type CompositeCandleProvider struct {
	universe        suPorts.SymbolUniverseProvider
	candleRepo      ports.CandleRepositoryPort
	exchangeInfoURL string
	tickerURL       string
}

// NewCompositeCandleProvider constructs the adapter.
func NewCompositeCandleProvider(
	universe suPorts.SymbolUniverseProvider,
	candleRepo ports.CandleRepositoryPort,
	exchangeInfoURL, tickerURL string,
) *CompositeCandleProvider {
	return &CompositeCandleProvider{
		universe:        universe,
		candleRepo:      candleRepo,
		exchangeInfoURL: exchangeInfoURL,
		tickerURL:       tickerURL,
	}
}

// Symbols implements metrics.CandleProvider.
func (p *CompositeCandleProvider) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	return p.universe.Symbols(ctx, p.exchangeInfoURL, p.tickerURL)
}

// GetLastNCandles implements metrics.CandleProvider.
func (p *CompositeCandleProvider) GetLastNCandles(ctx context.Context, symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error) {
	return p.candleRepo.GetLastNCandles(ctx, symbol, timeframe, n)
}
