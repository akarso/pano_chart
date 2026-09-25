package usecases

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/application/ports"
	appsignal "pano_chart/backend/application/signal"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring" // also used for structural regime detection (compression/breakout)
	domainsignal "pano_chart/backend/domain/signal"
)

// RankingsUseCase defines the boundary for the rankings v2 use case.
type RankingsUseCase interface {
	Execute(ctx context.Context, req GetRankingsRequest) (RankingsResult, error)
}

// RankingsResult is the full rankings response including RS metadata (PR-096).
type RankingsResult struct {
	Results       []RankedResult
	RSAvailable   bool     // true only when ≥1 row was scored against a usable tape
	Sort          SortMode // effective sort (may fall back from leaders/laggards)
	RequestedSort SortMode // sort from the request (before fallback)
}

// SortMode represents the sorting strategy for rankings.
type SortMode string

const (
	SortByTotal    SortMode = "total"
	SortByGain     SortMode = "gain"
	SortBySideways SortMode = "sideways"
	SortByTrend    SortMode = "trend"
	SortByVolume   SortMode = "volume"
	SortByLeaders  SortMode = "leaders"  // RS descending (PR-096)
	SortByLaggards SortMode = "laggards" // RS ascending (PR-096)
)

// ScoreKeyForSort maps sort modes to score calculator names.
// Leaders/laggards are not calculator scores — see sortValue.
var ScoreKeyForSort = map[SortMode]string{
	SortByGain:     "Gain/Loss",
	SortBySideways: "Sideways Consistency",
	SortByTrend:    "Trend Predictability",
}

// ParseSortMode converts a string to a SortMode, defaulting to SortByTotal.
func ParseSortMode(s string) SortMode {
	switch SortMode(s) {
	case SortByTotal, SortByGain, SortBySideways, SortByTrend, SortByVolume,
		SortByLeaders, SortByLaggards:
		return SortMode(s)
	default:
		return SortByTotal
	}
}

// CompositeTapeProvider supplies a market tape for relative-strength fields.
// Same shape as application/market.TapeProvider and signal.TapeSource.
// Optional on GetRankings — see SetTapeProvider.
type CompositeTapeProvider interface {
	CalculateTape(ctx context.Context, timeframe string, limit int) (metrics.CompositeTape, error)
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

	// SignalPrice / SignalATR are 14-period series ATR + last close for PR-090
	// badge logging. Not part of the public rankings JSON contract.
	SignalPrice float64
	SignalATR   float64

	// Relative strength vs the composite tape (PR-096).
	// Nil means unset (skipped / RS unavailable) — distinct from a real 0.
	RelativeStrength *float64 // ln(sym) − ln(tape) on timestamp overlap
	Beta             *float64 // OLS on the same aligned return pairs
	RSRank           *float64 // percentile among scored RS rows only (0–1)

	sparkTS []int64 // candle timestamps parallel to Sparkline (not JSON)
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

	snapshotLogger ports.SnapshotLogger  // optional; nil = no logging
	signalEmitter  ports.SignalEmitter   // optional; nil = no signal log (PR-090)
	tape           CompositeTapeProvider // optional; nil → RS unavailable (PR-096)
	rsFilter       symbolSkipper         // optional; composite.exclude names skip RS
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

// SetSignalEmitter attaches an optional signal logger (PR-090).
func (g *GetRankings) SetSignalEmitter(e ports.SignalEmitter) {
	g.signalEmitter = e
}

// SetTapeProvider attaches an optional composite tape source for RS/Beta/RSRank.
// When nil (default), RSAvailable stays false and leaders/laggards fall back to total.
func (g *GetRankings) SetTapeProvider(tp CompositeTapeProvider) {
	g.tape = tp
}

// SetRSFilter skips RS for names that match the composite exclude list
// (stables / wrappers). Pass nil to score every symbol.
func (g *GetRankings) SetRSFilter(f symbolSkipper) {
	g.rsFilter = f
}

// Execute computes the full ranking, annotates with volume, and sorts by mode.
func (g *GetRankings) Execute(ctx context.Context, req GetRankingsRequest) (RankingsResult, error) {
	empty := RankingsResult{Sort: req.Sort, RequestedSort: req.Sort}
	// 1. Resolve universe
	symbols, err := g.universe.Symbols(ctx, g.exchangeInfoURL, g.tickerURL)
	if err != nil {
		return empty, fmt.Errorf("universe fetch failed: %w", err)
	}
	if len(symbols) == 0 {
		return RankingsResult{
			Results:       []RankedResult{},
			Sort:          effectiveSort(req.Sort, false),
			RequestedSort: req.Sort,
		}, nil
	}

	// 2. Fetch volumes
	volMap, err := g.volumes.Volumes(ctx)
	if err != nil {
		return empty, fmt.Errorf("volume fetch failed: %w", err)
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
			ranked, err := ranker.Rank(gCtx, singleSeries)
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
		return empty, fmt.Errorf("parallel fetch+score failed: %w", err)
	}

	// 4. Build results with volume annotation and sparkline (+ timestamps for RS).
	results := make([]RankedResult, 0, len(symbols))
	for _, fr := range fetchResults {
		if !fr.hasSeries {
			continue
		}

		vol := volMap[fr.symbol.String()]

		all := fr.series.All()
		sparkline := make([]float64, len(all))
		sparkTS := make([]int64, len(all))
		for k, c := range all {
			sparkline[k] = c.Close()
			sparkTS[k] = c.Timestamp().Unix()
		}
		var signalPrice, signalATR float64
		if len(all) > 0 {
			signalPrice = all[len(all)-1].Close()
			signalATR = SimpleATR(fr.series, 14)
		}

		results = append(results, RankedResult{
			Symbol:      fr.ranked.Symbol,
			TotalScore:  fr.ranked.TotalScore,
			Scores:      fr.ranked.Scores,
			Volume:      vol,
			Sparkline:   sparkline,
			SignalPrice: signalPrice,
			SignalATR:   signalATR,
			sparkTS:     sparkTS,
		})
	}

	// 5. Relative strength vs composite tape (timestamp overlap).
	rsOK := g.annotateRelativeStrength(ctx, req.Timeframe, results)
	sortMode := effectiveSort(req.Sort, rsOK)

	// 6. Sort by effective mode (metric desc, then symbol asc).
	sortResults(results, sortMode)

	// 7. Percentile is position after the effective sort (not RSRank).
	n := len(results)
	for i := range results {
		if n <= 1 {
			results[i].Percentile = 1.0
		} else {
			results[i].Percentile = 1.0 - float64(i)/float64(n-1)
		}
	}

	// 8. Compute per-component percentiles + badge assignment.
	computeComponentPercentiles(results)
	assignBadges(results)
	g.emitBadgeSignals(ctx, req.Timeframe.String(), results)

	// 9. Sign-adjust trend score for directional display.
	signAdjustTrend(results)

	return RankingsResult{
		Results:       results,
		RSAvailable:   rsOK,
		Sort:          sortMode,
		RequestedSort: req.Sort,
	}, nil
}

func effectiveSort(requested SortMode, rsOK bool) SortMode {
	if !rsOK && (requested == SortByLeaders || requested == SortByLaggards) {
		return SortByTotal
	}
	return requested
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

func (g *GetRankings) emitBadgeSignals(ctx context.Context, timeframe string, results []RankedResult) {
	EmitBadgeSignals(ctx, g.signalEmitter, timeframe, results)
}

// EmitBadgeSignals records KindBadge for each result with a non-empty badge.
// Safe with a nil emitter. Used by GetRankings and by RedisCachedRankings on
// cache hits so user-facing badge calls are logged every candle, not only
// on cache miss.
func EmitBadgeSignals(ctx context.Context, emitter ports.SignalEmitter, timeframe string, results []RankedResult) {
	if emitter == nil {
		return
	}
	for _, r := range results {
		if r.BadgeComponent == "" {
			continue
		}
		label := appsignal.BadgeLabel(r.BadgeComponent, r.Sparkline)
		price := r.SignalPrice
		if price <= 0 && len(r.Sparkline) > 0 {
			price = r.Sparkline[len(r.Sparkline)-1]
		}
		ctxNums := map[string]float64{
			"total_score": r.TotalScore,
			"percentile":  r.Percentile,
		}
		if label == "sideways" {
			lo, hi := appsignal.SparklineRange(r.Sparkline)
			ctxNums["range_low"] = lo
			ctxNums["range_high"] = hi
		}
		emitter.Emit(ctx, domainsignal.Signal{
			Kind:      domainsignal.KindBadge,
			Symbol:    r.Symbol.String(),
			Timeframe: timeframe,
			Label:     label,
			Score:     r.MaxPercentile,
			Price:     price,
			ATR:       r.SignalATR, // 0 = flat / unavailable; still persisted
			Context:   ctxNums,
		})
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

// SidewaysCalcFor returns the sideways calculator for the given algo mode.
// V3 and V5 use TimeframeAwareCalculator so IdealATRRange / RMin-RMax match
// the series timeframe (not a hardcoded "1h"). Shared by main.go wiring and
// the rankings algo-override path (rankerForAlgo / swapSidewaysCalculator).
func SidewaysCalcFor(algo SidewaysAlgoMode) scoring.SymbolScoreCalculator {
	return sidewaysCalcFor(algo)
}

// sidewaysCalcFor returns the sideways calculator for the given algo mode.
// V3 and V5 use TimeframeAwareCalculator so IdealATRRange / RMin-RMax match
// the series timeframe (not a hardcoded "1h").
func sidewaysCalcFor(algo SidewaysAlgoMode) scoring.SymbolScoreCalculator {
	switch algo {
	case SidewaysAlgoV2:
		return &scoring.SidewaysV2ScoreCalculator{}
	case SidewaysAlgoV3:
		return scoring.NewTimeframeAwareCalculator("Sideways Consistency", func(tf string) scoring.SymbolScoreCalculator {
			return &scoring.SidewaysV3ScoreCalculator{
				Config: scoring.DefaultSidewaysV3Config(tf),
			}
		})
	case SidewaysAlgoV4:
		return &scoring.SidewaysV4ScoreCalculator{}
	case SidewaysAlgoV5:
		return scoring.NewTimeframeAwareCalculator("Sideways Consistency", func(tf string) scoring.SymbolScoreCalculator {
			return &scoring.SidewaysV5ScoreCalculator{
				Config: scoring.NewSidewaysV5ConfigForTimeframe(tf),
			}
		})
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
// Primary: selected metric descending. Secondary: symbol ascending.
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
	case SortByLeaders:
		return rsSortKey(r, false)
	case SortByLaggards:
		return rsSortKey(r, true)
	default:
		key, ok := ScoreKeyForSort[mode]
		if !ok {
			return r.TotalScore
		}
		return r.Scores[key]
	}
}

// annotateRelativeStrength fills RS fields from timestamp overlap with the tape.
// Returns true only when a usable tape yielded at least one scored row.
// Nil provider is silent (RS disabled by design); tape errors/short/zero-scored log.
func (g *GetRankings) annotateRelativeStrength(ctx context.Context, tf domain.Timeframe, results []RankedResult) bool {
	if g.tape == nil || len(results) == 0 {
		return false
	}
	tape, err := g.tape.CalculateTape(ctx, tf.String(), metrics.CompositeTapeWindow)
	if err != nil {
		log.Printf("[rankings] relative strength: tape unavailable: %v", err)
		return false
	}
	byTS := tapeCloseByTS(tape)
	if len(byTS) < 2 {
		log.Printf("[rankings] relative strength: tape too short (%d stamps)", len(byTS))
		return false
	}
	scored := applyRelativeStrength(results, byTS, g.rsFilter)
	if scored == 0 {
		log.Printf("[rankings] relative strength: usable tape but zero scored rows (overlap/exclude)")
		return false
	}
	return true
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
