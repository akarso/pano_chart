package usecases

import (
	"context"
	"pano_chart/backend/application/symbol_universe"
	"pano_chart/backend/domain"
)

// RankSymbolsWithUniverse wires RankSymbols with a SymbolUniverseProvider.
type RankSymbolsWithUniverse struct {
	Ranker   RankSymbols
	Universe symbol_universe.SymbolUniverseProvider
}

func NewRankSymbolsWithUniverse(ranker RankSymbols, universe symbol_universe.SymbolUniverseProvider) *RankSymbolsWithUniverse {
	return &RankSymbolsWithUniverse{
		Ranker:   ranker,
		Universe: universe,
	}
}

// RankAll fetches symbols from the universe and ranks them.
func (r *RankSymbolsWithUniverse) RankAll(ctx context.Context, exchangeInfoURL, tickerURL string, series map[domain.Symbol]domain.CandleSeries) ([]RankedSymbol, error) {
	syms, err := r.Universe.Symbols(ctx, exchangeInfoURL, tickerURL)
	if err != nil {
		return nil, err
	}
	filtered := make(map[domain.Symbol]domain.CandleSeries)
	for _, sym := range syms {
		if s, ok := series[sym]; ok {
			filtered[sym] = s
		}
	}
	return r.Ranker.Rank(filtered)
}
