package infra

import (
	"sort"
	"pano_chart/backend/domain"
)

type SymbolUniversePort interface {
	ListSymbols() ([]domain.Symbol, error)
}

type StaticSymbolUniverse struct {
	symbols []domain.Symbol
}

func NewStaticSymbolUniverse(symbols []string) (*StaticSymbolUniverse, error) {
	dedup := map[string]struct{}{}
	var norm []domain.Symbol
	for _, s := range symbols {
		sy, err := domain.NewSymbol(s)
		if err != nil {
			return nil, err
		}
		if _, ok := dedup[sy.String()]; !ok {
			dedup[sy.String()] = struct{}{}
			norm = append(norm, sy)
		}
	}
	sort.Slice(norm, func(i, j int) bool { return norm[i] < norm[j] })
	return &StaticSymbolUniverse{symbols: norm}, nil
}

func (s *StaticSymbolUniverse) ListSymbols() ([]domain.Symbol, error) {
	return append([]domain.Symbol(nil), s.symbols...), nil
}
