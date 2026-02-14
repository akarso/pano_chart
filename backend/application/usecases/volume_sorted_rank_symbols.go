package usecases

import (
	"context"
	"fmt"
	"pano_chart/backend/domain"
	"sort"
)

type VolumeProvider interface {
	Volumes(ctx context.Context) (map[string]float64, error)
}

type SymbolUniverseProvider interface {
	Symbols(ctx context.Context, exchangeInfoURL, tickerURL string) ([]domain.Symbol, error)
}

type VolumeSortedRankSymbols struct {
	universe        SymbolUniverseProvider
	Volumes         VolumeProvider
	Weights         []ScoreWeight
	ExchangeInfoURL string
	TickerURL       string
}

func NewVolumeSortedRankSymbols(universe SymbolUniverseProvider, volumes VolumeProvider, weights []ScoreWeight, exchangeInfoURL, tickerURL string) *VolumeSortedRankSymbols {
	return &VolumeSortedRankSymbols{
		universe:        universe,
		Volumes:         volumes,
		Weights:         weights,
		ExchangeInfoURL: exchangeInfoURL,
		TickerURL:       tickerURL,
	}
}

// Universe returns the SymbolUniverseProvider for this ranker.
func (v *VolumeSortedRankSymbols) Universe() SymbolUniverseProvider {
	return v.universe
}

func (v *VolumeSortedRankSymbols) Rank(series map[domain.Symbol]domain.CandleSeries) ([]RankedSymbol, error) {
	ctx := context.Background()
	if v.ExchangeInfoURL == "" || v.TickerURL == "" {
		return nil, fmt.Errorf("exchangeInfo and ticker URLs are required")
	}
	syms, err := v.universe.Symbols(ctx, v.ExchangeInfoURL, v.TickerURL)
	if err != nil {
		return nil, err
	}
	volMap, err := v.Volumes.Volumes(ctx)
	if err != nil {
		return nil, err
	}
	// Only rank symbols present in both universe and volume map
	var filtered []domain.Symbol
	for _, s := range syms {
		if _, ok := volMap[s.String()]; ok {
			filtered = append(filtered, s)
		}
	}
	// Sort by descending volume, then alphabetically
	sort.Slice(filtered, func(i, j int) bool {
		vi, vj := volMap[filtered[i].String()], volMap[filtered[j].String()]
		if vi == vj {
			return filtered[i].String() < filtered[j].String()
		}
		return vi > vj
	})
	// Build series map for filtered symbols
	filteredSeries := make(map[domain.Symbol]domain.CandleSeries)
	for _, s := range filtered {
		if cs, ok := series[s]; ok {
			filteredSeries[s] = cs
		}
	}
	// Use DefaultRankSymbols logic for scoring
	base := NewDefaultRankSymbols(v.Weights)
	return base.Rank(filteredSeries)
}
