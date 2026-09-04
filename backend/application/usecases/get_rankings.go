package usecases

import (
	"context"
	"fmt"
	"math"
	"sort"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring" // also used for structural regime detection (compression/breakout)
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
	SidewaysAlgoV3 SidewaysAlgoMode = "v3"
	SidewaysAlgoV4 SidewaysAlgoMode = "v4"
	SidewaysAlgoV5 SidewaysAlgoMode = "v5"
)

// ParseSidewaysAlgo normalises a string to a valid SidewaysAlgoMode.
// Empty string means "use the configured default".
func ParseSidewaysAlgo(s string) SidewaysAlgoMode {
	switch SidewaysAlgoMode(s) {
	case SidewaysAlgoV1, SidewaysAlgoV2, SidewaysAlgoV3, SidewaysAlgoV4, SidewaysAlgoV5:
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
	Percentile float64
	Scores     map[string]float64
	Volume     float64
	Sparkline  []float64

	// Per-component percentiles (position-based, computed across full universe).
	TrendPercentile    float64
	SidewaysPercentile float64
	GainPercentile     float64

	// Derived from per-component percentiles.
	MaxPercentile     float64
	DominantComponent string // "trend", "sideways", or "gain"
	BadgeComponent    string // same as DominantComponent for Top-N, empty otherwise
}

// GetRankings computes full ranked results for the universe.
// It fetches universe, volumes, candle series for each symbol, scores them,
// and sorts by the requested mode.
type GetRankings struct {
	universe    SymbolUniverseProvider
	ranker      RankSymbols
	volumes     VolumeProvider
	candleRepo  ports.CandleRepositoryPort
	precision   int
	defaultAlgo SidewaysAlgoMode
	weights     []ScoreWeight
	workerLimit int64

	exchangeInfoURL string
	tickerURL       string

	snapshotLogger ports.SnapshotLogger // optional; nil = no logging
}

// NewGetRankings constructs the use case.
// snapshotLogger is optional — pass nil to disable evaluation logging.
func NewGetRankings(
	universe SymbolUniverseProvider,
	ranker RankSymbols,
	volumes VolumeProvider,
	candleRepo ports.CandleRepositoryPort,
	exchangeInfoURL, tickerURL string,
	precision int,
	defaultAlgo SidewaysAlgoMode,
	weights []ScoreWeight,
	workerLimit int,
	snapshotLogger ports.SnapshotLogger,
) *GetRankings {
	if precision <= 0 {
		precision = 110
	}
	if defaultAlgo == "" {
		defaultAlgo = SidewaysAlgoV5
	}
	wl := int64(workerLimit)
	if wl <= 0 {
		wl = 12
	}
	return &GetRankings{
		universe:        universe,
		ranker:          ranker,
		volumes:         volumes,
		candleRepo:      candleRepo,
		precision:       precision,
		defaultAlgo:     defaultAlgo,
		weights:         weights,
		workerLimit:     wl,
		exchangeInfoURL: exchangeInfoURL,
		tickerURL:       tickerURL,
		snapshotLogger:  snapshotLogger,
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

	// 3. Fetch candles + score each symbol in a bounded worker pool.
	//    Each worker fetches candles and scores the symbol independently.
	//    Results are written by index — no mutex needed for the slice.
	ranker := g.rankerForAlgo(req.SidewaysAlgo)
	type fetchResult struct {
		symbol    domain.Symbol
		series    domain.CandleSeries
		ranked    RankedSymbol
		hasSeries bool
	}

	sem := semaphore.NewWeighted(g.workerLimit)
	grp, gCtx := errgroup.WithContext(ctx)

	fetchResults := make([]fetchResult, len(symbols))
	for i, sym := range symbols {
		i, sym := i, sym

		if err := sem.Acquire(gCtx, 1); err != nil {
			break // context cancelled
		}

		grp.Go(func() error {
			defer sem.Release(1)

			cs, err := g.candleRepo.GetLastNCandles(gCtx, sym, req.Timeframe, g.precision)
			if err != nil {
				return nil // skip symbols with fetch errors (partial failure tolerance)
			}

			// Score inline — avoids building a full map and re-iterating.
			singleSeries := map[domain.Symbol]domain.CandleSeries{sym: cs}
			ranked, err := ranker.Rank(singleSeries)
			if err != nil || len(ranked) == 0 {
				return nil // skip on scoring error
			}

			// Run structural regime detectors (compression → breakout)
			// and inject into scores map. These do NOT affect TotalScore.
			candles := cs.All()
			compResult := scoring.DetectCompression(candles, scoring.DefaultCompressionConfig())
			breakResult := scoring.DetectBreakout(candles, scoring.DefaultBreakoutConfig(), compResult.Score)
			ranked[0].Scores["Compression"] = compResult.Score
			ranked[0].Scores["Breakout Up"] = breakResult.UpScore
			ranked[0].Scores["Breakout Down"] = breakResult.DownScore

			fetchResults[i] = fetchResult{
				symbol:    sym,
				series:    cs,
				ranked:    ranked[0],
				hasSeries: true,
			}

			// Log evaluation snapshot (fire-and-forget, non-blocking).
			if g.snapshotLogger != nil {
				snap := BuildSnapshot(sym, req.Timeframe, ranked[0].Scores, cs, 0, domain.AlgoVersion)
				_ = g.snapshotLogger.Log(snap)
			}

			return nil
		})
	}

	if err := grp.Wait(); err != nil {
		return nil, fmt.Errorf("parallel fetch+score failed: %w", err)
	}

	// 4. Build results with volume annotation and sparkline.
	//    Collect only successful results (partial failure tolerance).
	results := make([]RankedResult, 0, len(symbols))
	for _, fr := range fetchResults {
		if !fr.hasSeries {
			continue
		}

		vol := volMap[fr.symbol.String()]

		// Extract sparkline from already-fetched series
		all := fr.series.All()
		sparkline := make([]float64, len(all))
		for k, c := range all {
			sparkline[k] = c.Close()
		}

		results = append(results, RankedResult{
			Symbol:     fr.ranked.Symbol,
			TotalScore: fr.ranked.TotalScore,
			Scores:     fr.ranked.Scores,
			Volume:     vol,
			Sparkline:  sparkline,
		})
	}

	// 5. Sort by requested mode (deterministic: metric desc, symbol asc)
	sortResults(results, req.Sort)

	// 6. Compute percentile rank based on position after sorting.
	//    Top symbol → 1.0, bottom → 0.0, single symbol → 1.0.
	n := len(results)
	for i := range results {
		if n <= 1 {
			results[i].Percentile = 1.0
		} else {
			results[i].Percentile = 1.0 - float64(i)/float64(n-1)
		}
	}

	// 7. Compute per-component percentiles + badge assignment.
	computeComponentPercentiles(results)
	assignBadges(results)

	// 8. Sign-adjust trend score for directional display.
	//    Positive = uptrend, negative = downtrend.
	//    Applied AFTER TotalScore, percentiles, and badges so they remain
	//    based on absolute trendiness.  The frontend re-sorts locally using
	//    the signed value so "trend up" puts uptrends first.
	signAdjustTrend(results)

	return results, nil
}

// computeComponentPercentiles ranks symbols per component score (desc)
// and assigns position-based percentiles for trend, sideways, and gain.
func computeComponentPercentiles(results []RankedResult) {
	n := len(results)
	if n == 0 {
		return
	}

	type componentDef struct {
		scoreKey string                     // key in Scores map
		setter   func(idx int, pct float64) // sets the percentile on results[idx]
	}

	components := []componentDef{
		{
			scoreKey: "Trend Predictability",
			setter:   func(i int, p float64) { results[i].TrendPercentile = p },
		},
		{
			scoreKey: "Sideways Consistency",
			setter:   func(i int, p float64) { results[i].SidewaysPercentile = p },
		},
		{
			scoreKey: "Gain/Loss",
			setter:   func(i int, p float64) { results[i].GainPercentile = p },
		},
	}

	// Build index slice once; reuse for each component sort.
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}

	for _, comp := range components {
		key := comp.scoreKey

		// Sort indices by this component score descending, tie-break by symbol asc.
		sort.Slice(indices, func(a, b int) bool {
			sa := results[indices[a]].Scores[key]
			sb := results[indices[b]].Scores[key]
			if sa != sb {
				return sa > sb
			}
			return results[indices[a]].Symbol.String() < results[indices[b]].Symbol.String()
		})

		for rank, idx := range indices {
			if n <= 1 {
				comp.setter(idx, 1.0)
			} else {
				comp.setter(idx, 1.0-float64(rank)/float64(n-1))
			}
		}
	}

	// Compute MaxPercentile and DominantComponent.
	for i := range results {
		tp := results[i].TrendPercentile
		sp := results[i].SidewaysPercentile
		gp := results[i].GainPercentile

		maxP := tp
		dom := "trend"
		if sp > maxP {
			maxP = sp
			dom = "sideways"
		}
		if gp > maxP {
			maxP = gp
			dom = "gain"
		}
		results[i].MaxPercentile = maxP
		results[i].DominantComponent = dom
	}
}

// assignBadges gives a badge to the Top-N symbols by MaxPercentile.
// TopN = max(1, ceil(N * 0.2)), capped at 10. N <= 1 means no badges.
func assignBadges(results []RankedResult) {
	n := len(results)
	if n <= 1 {
		return
	}

	topN := int(math.Ceil(float64(n) * 0.2))
	if topN < 1 {
		topN = 1
	}
	if topN > 10 {
		topN = 10
	}

	// Build index slice sorted by MaxPercentile desc, tie-break symbol asc.
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		ma := results[indices[a]].MaxPercentile
		mb := results[indices[b]].MaxPercentile
		if ma != mb {
			return ma > mb
		}
		return results[indices[a]].Symbol.String() < results[indices[b]].Symbol.String()
	})

	for rank, idx := range indices {
		if rank < topN {
			results[idx].BadgeComponent = results[idx].DominantComponent
		}
	}
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
	case SidewaysAlgoV3:
		return &scoring.SidewaysV3ScoreCalculator{
			Config: scoring.DefaultSidewaysV3Config("1h"),
		}
	case SidewaysAlgoV4:
		return &scoring.SidewaysV4ScoreCalculator{}
	case SidewaysAlgoV5:
		return &scoring.SidewaysV5ScoreCalculator{
			Config: scoring.NewSidewaysV5ConfigForTimeframe("1h"),
		}
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
			return vi > vj // descending by metric (full precision)
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

// signAdjustTrend negates the "Trend Predictability" score for symbols whose
// sparkline slopes downward (last close < first close).  The result is a
// signed score: positive = uptrend, negative = downtrend.
//
// Call this AFTER TotalScore, percentiles, and badges have been computed so
// those remain based on absolute trendiness.
func signAdjustTrend(results []RankedResult) {
	const key = "Trend Predictability"
	for i := range results {
		ts := results[i].Scores[key]
		if ts == 0 {
			continue
		}
		sp := results[i].Sparkline
		if len(sp) < 2 {
			continue
		}
		if sp[len(sp)-1] < sp[0] {
			results[i].Scores[key] = -ts
		}
	}
}
