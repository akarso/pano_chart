package usecases

import (
	"context"
	"fmt"
	"sort"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
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

// SidewaysAlgoMode identifies which sideways scoring algorithm to use.
type SidewaysAlgoMode string

const (
	SidewaysAlgoV1 SidewaysAlgoMode = "v1"
	SidewaysAlgoV2 SidewaysAlgoMode = "v2"
)

// ParseSidewaysAlgo normalises a string to a valid SidewaysAlgoMode.
// Empty string means "use the configured default".
func ParseSidewaysAlgo(s string) SidewaysAlgoMode {
	switch SidewaysAlgoMode(s) {
	case SidewaysAlgoV1, SidewaysAlgoV2:
		return SidewaysAlgoMode(s)
	default:
		return "" // use default
	}
}

// GetRankingsRequest encapsulates the input for the rankings use case.
type GetRankingsRequest struct {
	Timeframe    domain.Timeframe
	Sort         SortMode
	SidewaysAlgo SidewaysAlgoMode // empty = use default
}

// RankedResult represents a single symbol in the rankings output.
type RankedResult struct {
	Symbol     domain.Symbol
	TotalScore float64
	Scores     map[string]float64
	Volume     float64
	Sparkline  []float64
}

// GetRankings computes full ranked results for the universe.
// It fetches universe, volumes, candle series for each symbol, scores them,
// and sorts by the requested mode.
type GetRankings struct {
	universe       SymbolUniverseProvider
	ranker         RankSymbols
	volumes        VolumeProvider
	candleRepo     ports.CandleRepositoryPort
	precision      int
	defaultAlgo    SidewaysAlgoMode
	weights        []ScoreWeight

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
	precision int,
	defaultAlgo SidewaysAlgoMode,
	weights []ScoreWeight,
) *GetRankings {
	if precision <= 0 {
		precision = 30
	}
	if defaultAlgo == "" {
		defaultAlgo = SidewaysAlgoV1
	}
	return &GetRankings{
		universe:        universe,
		ranker:          ranker,
		volumes:         volumes,
		candleRepo:      candleRepo,
		precision:       precision,
		defaultAlgo:     defaultAlgo,
		weights:         weights,
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

	// 3. Build candle series for all symbols using GetLastNCandles
	//    (GetSeries with zero times produces invalid Binance URLs)
	series := make(map[domain.Symbol]domain.CandleSeries)
	for _, sym := range symbols {
		cs, err := g.candleRepo.GetLastNCandles(sym, req.Timeframe, g.precision)
		if err != nil {
			continue // skip symbols with fetch errors
		}
		series[sym] = cs
	}

	// 4. Score all symbols using the ranker (optionally with algo override)
	ranker := g.rankerForAlgo(req.SidewaysAlgo)
	ranked, err := ranker.Rank(series)
	if err != nil {
		return nil, fmt.Errorf("ranking failed: %w", err)
	}

	// 5. Build results with volume annotation and sparkline (reuse series from step 3)
	results := make([]RankedResult, 0, len(ranked))
	for _, r := range ranked {
		vol := volMap[r.Symbol.String()]

		// Extract sparkline from already-fetched series
		var sparkline []float64
		if cs, ok := series[r.Symbol]; ok {
			all := cs.All()
			sparkline = make([]float64, len(all))
			for k, c := range all {
				sparkline[k] = c.Close()
			}
		}

		results = append(results, RankedResult{
			Symbol:     r.Symbol,
			TotalScore: r.TotalScore,
			Scores:     r.Scores,
			Volume:     vol,
			Sparkline:  sparkline,
		})
	}

	// 6. Sort by requested mode (deterministic: metric desc, symbol asc)
	sortResults(results, req.Sort)

	return results, nil
}

// rankerForAlgo returns the ranker to use for the given algo override.
// If override is empty, it uses the configured default.
// When the effective algo differs from the ranker's compiled-in calculator,
// a new ranker with the swapped sideways calculator is created.
func (g *GetRankings) rankerForAlgo(override SidewaysAlgoMode) RankSymbols {
	effective := override
	if effective == "" {
		effective = g.defaultAlgo
	}

	// The ranker was built with the default algo. If effective matches default, reuse it.
	if effective == g.defaultAlgo {
		return g.ranker
	}

	// Build a new DefaultRankSymbols with swapped sideways calculator.
	swapped := swapSidewaysCalculator(g.weights, effective)
	return NewDefaultRankSymbols(swapped)
}

// sidewaysCalcFor returns the sideways calculator for the given algo mode.
func sidewaysCalcFor(algo SidewaysAlgoMode) scoring.SymbolScoreCalculator {
	switch algo {
	case SidewaysAlgoV2:
		return &scoring.SidewaysV2ScoreCalculator{}
	default:
		return &scoring.SidewaysConsistencyScoreCalculator{}
	}
}

// swapSidewaysCalculator returns a copy of weights with the sideways calculator
// replaced by the one matching the given algo.
func swapSidewaysCalculator(weights []ScoreWeight, algo SidewaysAlgoMode) []ScoreWeight {
	calc := sidewaysCalcFor(algo)
	out := make([]ScoreWeight, len(weights))
	copy(out, weights)
	for i := range out {
		if out[i].Calculator.Name() == "Sideways Consistency" {
			out[i].Calculator = calc
		}
	}
	return out
}

// sortResults sorts results in-place by the given mode.
// Primary: selected metric descending. Secondary: volume descending. Tertiary: symbol ascending.
func sortResults(results []RankedResult, mode SortMode) {
	sort.SliceStable(results, func(i, j int) bool {
		vi := sortValue(results[i], mode)
		vj := sortValue(results[j], mode)
		if vi != vj {
			return vi > vj // descending by metric
		}
		if results[i].Volume != results[j].Volume {
			return results[i].Volume > results[j].Volume // descending by volume
		}
		return results[i].Symbol.String() < results[j].Symbol.String() // ascending alphabetical
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
