package usecases

import (
	"context"
	"fmt"
	"sort"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// RankingsUseCase defines the boundary for the rankings v2 use case.
type RankingsUseCase interface {
	Execute(ctx context.Context, req GetRankingsRequest) ([]RankedResult, error)
}

// SortMode represents the sorting strategy for rankings.
type SortMode string

const (
	SortByTotal    SortMode = "total"
	SortByGain     SortMode = "gain"
	SortBySideways SortMode = "sideways"
	SortByTrend    SortMode = "trend"
	SortByVolume   SortMode = "volume"
)

// ScoreKeyForSort maps sort modes to score calculator names.
var ScoreKeyForSort = map[SortMode]string{
	SortByGain:     "Gain/Loss",
	SortBySideways: "Sideways Consistency",
	SortByTrend:    "Trend Predictability",
}

// ParseSortMode converts a string to a SortMode, defaulting to SortByTotal.
func ParseSortMode(s string) SortMode {
	switch SortMode(s) {
	case SortByTotal, SortByGain, SortBySideways, SortByTrend, SortByVolume:
		return SortMode(s)
	default:
		return SortByTotal
	}
}

// GetRankingsRequest encapsulates the input for the rankings use case.
type GetRankingsRequest struct {
	Timeframe domain.Timeframe
	Sort      SortMode
}

// RankedResult represents a single symbol in the rankings output.
type RankedResult struct {
	Symbol     domain.Symbol
	TotalScore float64
	Scores     map[string]float64
	Volume     float64
}

// GetRankings computes full ranked results for the universe.
// It fetches universe, volumes, candle series for each symbol, scores them,
// and sorts by the requested mode.
type GetRankings struct {
	universe   SymbolUniverseProvider
	ranker     RankSymbols
	volumes    VolumeProvider
	candleRepo ports.CandleRepositoryPort

	exchangeInfoURL string
	tickerURL       string
}

// NewGetRankings constructs the use case.
func NewGetRankings(
	universe SymbolUniverseProvider,
	ranker RankSymbols,
	volumes VolumeProvider,
	candleRepo ports.CandleRepositoryPort,
	exchangeInfoURL, tickerURL string,
) *GetRankings {
	return &GetRankings{
		universe:        universe,
		ranker:          ranker,
		volumes:         volumes,
		candleRepo:      candleRepo,
		exchangeInfoURL: exchangeInfoURL,
		tickerURL:       tickerURL,
	}
}

// Execute computes the full ranking, annotates with volume, and sorts by mode.
func (g *GetRankings) Execute(ctx context.Context, req GetRankingsRequest) ([]RankedResult, error) {
	// 1. Resolve universe
	symbols, err := g.universe.Symbols(ctx, g.exchangeInfoURL, g.tickerURL)
	if err != nil {
		return nil, fmt.Errorf("universe fetch failed: %w", err)
	}
	if len(symbols) == 0 {
		return []RankedResult{}, nil
	}

	// 2. Fetch volumes
	volMap, err := g.volumes.Volumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("volume fetch failed: %w", err)
	}

	// 3. Build candle series for all symbols
	series := make(map[domain.Symbol]domain.CandleSeries)
	for _, sym := range symbols {
		cs, err := g.candleRepo.GetSeries(sym, req.Timeframe, time.Time{}, time.Time{})
		if err != nil {
			continue // skip symbols with fetch errors
		}
		series[sym] = cs
	}

	// 4. Score all symbols using the ranker
	ranked, err := g.ranker.Rank(series)
	if err != nil {
		return nil, fmt.Errorf("ranking failed: %w", err)
	}

	// 5. Build results with volume annotation
	results := make([]RankedResult, 0, len(ranked))
	for _, r := range ranked {
		vol := volMap[r.Symbol.String()]
		results = append(results, RankedResult{
			Symbol:     r.Symbol,
			TotalScore: r.TotalScore,
			Scores:     r.Scores,
			Volume:     vol,
		})
	}

	// 6. Sort by requested mode (deterministic: metric desc, symbol asc)
	sortResults(results, req.Sort)

	return results, nil
}

// sortResults sorts results in-place by the given mode.
// Primary: selected metric descending. Secondary: symbol ascending.
func sortResults(results []RankedResult, mode SortMode) {
	sort.SliceStable(results, func(i, j int) bool {
		vi := sortValue(results[i], mode)
		vj := sortValue(results[j], mode)
		if vi != vj {
			return vi > vj // descending
		}
		return results[i].Symbol.String() < results[j].Symbol.String() // ascending
	})
}

// sortValue extracts the metric value used for sorting.
func sortValue(r RankedResult, mode SortMode) float64 {
	switch mode {
	case SortByTotal:
		return r.TotalScore
	case SortByVolume:
		return r.Volume
	default:
		key, ok := ScoreKeyForSort[mode]
		if !ok {
			return r.TotalScore
		}
		return r.Scores[key]
	}
}
