package usecases

import (
	"context"
	"pano_chart/backend/domain"
	"sort"
)

type VolumeProvider interface {
	Volumes(ctx context.Context) (map[string]float64, error)
}

type SymbolUniverseProvider interface {
	Symbols(ctx context.Context) ([]domain.Symbol, error)
}

type VolumeSortedRankSymbols struct {
	universe SymbolUniverseProvider
	Volumes  VolumeProvider
	Weights  []ScoreWeight
}

func NewVolumeSortedRankSymbols(universe SymbolUniverseProvider, volumes VolumeProvider, weights []ScoreWeight) *VolumeSortedRankSymbols {
	return &VolumeSortedRankSymbols{universe: universe, Volumes: volumes, Weights: weights}
}

// Universe returns the SymbolUniverseProvider for this ranker.
func (v *VolumeSortedRankSymbols) Universe() SymbolUniverseProvider {
	return v.universe
}

func (v *VolumeSortedRankSymbols) Rank(series map[domain.Symbol]domain.CandleSeries) ([]RankedSymbol, error) {
	ctx := context.Background()
	syms, err := v.universe.Symbols(ctx)
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
