package metrics

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// CandleProvider provides candle data and symbol lists for market metrics.
type CandleProvider interface {
	// Symbols returns the current symbol universe.
	Symbols(ctx context.Context) ([]domain.Symbol, error)
	// GetLastNCandles retrieves the last N candles for a symbol and timeframe.
	GetLastNCandles(ctx context.Context, symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error)
}

// CompositeIndexService computes a normalized composite market index (PR-095):
// timestamp alignment (≥50% coverage), log-return aggregation, and symbol exclusions.
type CompositeIndexService struct {
	provider    CandleProvider
	workerLimit int
	filter      *SymbolFilter
}

// NewCompositeIndexService constructs the service with default exclusions.
func NewCompositeIndexService(p CandleProvider, workerLimit int) *CompositeIndexService {
	return NewCompositeIndexServiceFiltered(p, workerLimit, DefaultSymbolFilter())
}

// NewCompositeIndexServiceFiltered constructs the service with an explicit filter.
// Pass nil to disable exclusions (tests that need the full fake universe).
func NewCompositeIndexServiceFiltered(p CandleProvider, workerLimit int, filter *SymbolFilter) *CompositeIndexService {
	if workerLimit <= 0 {
		workerLimit = 20
	}
	return &CompositeIndexService{provider: p, workerLimit: workerLimit, filter: filter}
}

// CompositeTape holds both index paths plus synthetic candle series suitable
// for the same per-chart scorers used on the rankings page.
type CompositeTape struct {
	Index           mkt.CompositeIndex
	MedianSeries    domain.CandleSeries
	WeightedSeries  domain.CandleSeries
	PreferredSource string // "composite_volume_weighted" or "composite_median"
}

// PreferredSeries returns the volume-weighted series when it has enough bars,
// otherwise the median series.
func (t CompositeTape) PreferredSeries() domain.CandleSeries {
	if t.PreferredSource == "composite_volume_weighted" && t.WeightedSeries.Len() >= 2 {
		return t.WeightedSeries
	}
	return t.MedianSeries
}

// Calculate produces a composite index for the given timeframe with at most
// `limit` data points (median + volume-weighted).
func (s *CompositeIndexService) Calculate(ctx context.Context, timeframe string, limit int) (mkt.CompositeIndex, error) {
	tape, err := s.CalculateTape(ctx, timeframe, limit)
	if err != nil {
		return mkt.CompositeIndex{}, err
	}
	return tape.Index, nil
}

type barOHLCV struct {
	open, high, low, close, volume float64
}

func (b barOHLCV) positive() bool {
	return b.open > 0 && b.high > 0 && b.low > 0 && b.close > 0
}

type symbolBars struct {
	byTS    map[int64]barOHLCV
	ordered []int64 // ascending timestamps with positive close (lazy)
	weight  float64 // quote-volume over the fetched window
}

// closeBefore returns the last positive close strictly before ts.
// Uses a sorted predecessor index so each lookup is O(log n) after one build.
func (p *symbolBars) closeBefore(ts int64) (float64, bool) {
	if p == nil || len(p.byTS) == 0 {
		return 0, false
	}
	p.ensureOrdered()
	i := sort.Search(len(p.ordered), func(i int) bool { return p.ordered[i] >= ts })
	if i == 0 {
		return 0, false
	}
	b := p.byTS[p.ordered[i-1]]
	return b.close, true
}

func (p *symbolBars) ensureOrdered() {
	if p.ordered != nil {
		return
	}
	ordered := make([]int64, 0, len(p.byTS))
	for t, b := range p.byTS {
		if b.close > 0 {
			ordered = append(ordered, t)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	p.ordered = ordered
}

// CalculateTape produces both composite paths and synthetic OHLCV series.
func (s *CompositeIndexService) CalculateTape(ctx context.Context, timeframe string, limit int) (CompositeTape, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return CompositeTape{}, err
	}
	if limit <= 0 {
		limit = 200
	}

	symbols, err := s.provider.Symbols(ctx)
	if err != nil {
		return CompositeTape{}, err
	}
	if len(symbols) == 0 {
		return emptyTape(timeframe, 0), nil
	}

	paths := s.fetchSymbolBars(ctx, symbols, tf, limit)
	if len(paths) == 0 {
		return emptyTape(timeframe, 0), nil
	}
	return assembleTape(timeframe, tf, paths, limit), nil
}

func emptyTape(timeframe string, n int) CompositeTape {
	return CompositeTape{
		Index:           mkt.CompositeIndex{Timeframe: timeframe, SymbolCount: n},
		PreferredSource: "composite_median",
	}
}

// fetchSymbolBars fans out candle fetches and builds per-symbol bar maps.
func (s *CompositeIndexService) fetchSymbolBars(
	ctx context.Context,
	symbols []domain.Symbol,
	tf domain.Timeframe,
	limit int,
) map[string]*symbolBars {
	var mu sync.Mutex
	paths := make(map[string]*symbolBars)
	sem := make(chan struct{}, s.workerLimit)
	var wg sync.WaitGroup

	for _, sym := range symbols {
		if s.filter != nil && s.filter.Skip(sym.String()) {
			continue
		}
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cs, fetchErr := s.provider.GetLastNCandles(ctx, sym, tf, limit)
			if fetchErr != nil || cs.Len() < 2 {
				return
			}
			candles := cs.All()
			byTS := make(map[int64]barOHLCV, len(candles))
			var quoteVol float64
			for _, c := range candles {
				b := barOHLCV{
					open: c.Open(), high: c.High(), low: c.Low(),
					close: c.Close(), volume: c.Volume(),
				}
				if !b.positive() {
					continue
				}
				byTS[c.Timestamp().Unix()] = b
				quoteVol += b.volume * b.close
			}
			if len(byTS) < 2 {
				return
			}
			if quoteVol <= 0 {
				quoteVol = 1
			}
			mu.Lock()
			paths[sym.String()] = &symbolBars{byTS: byTS, weight: quoteVol}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return paths
}

// assembleTape selects the active set, builds the reference timeline, and
// aggregates median + volume-weighted synthetic series.
func assembleTape(timeframe string, tf domain.Timeframe, paths map[string]*symbolBars, limit int) CompositeTape {
	active := activePaths(paths)
	if len(active) == 0 {
		active = paths
	}
	ref := referenceTimeline(active, limit)
	if len(ref) == 0 {
		return emptyTape(timeframe, len(active))
	}

	medianPts, weightedPts, medianCandles, weightedCandles := aggregateAlongTimeline(active, tf, ref)

	medianSeries, _ := domain.NewCandleSeries(domain.NewSymbolUnsafe("COMPOSITE"), tf, medianCandles)
	weightedSeries, _ := domain.NewCandleSeries(domain.NewSymbolUnsafe("COMPOSITE"), tf, weightedCandles)

	source := "composite_median"
	if weightedSeries.Len() >= 2 {
		source = "composite_volume_weighted"
	}

	return CompositeTape{
		Index: mkt.CompositeIndex{
			Timeframe:            timeframe,
			Points:               medianPts,
			VolumeWeightedPoints: weightedPts,
			SymbolCount:          len(active),
		},
		MedianSeries:    medianSeries,
		WeightedSeries:  weightedSeries,
		PreferredSource: source,
	}
}

// aggregateAlongTimeline walks the reference timestamps and emits index points
// plus synthetic OHLCV candles for both median and volume-weighted paths.
func aggregateAlongTimeline(
	active map[string]*symbolBars,
	tf domain.Timeframe,
	ref []int64,
) (medianPts, weightedPts []mkt.IndexPoint, medianCandles, weightedCandles []domain.Candle) {
	synthSym := domain.NewSymbolUnsafe("COMPOSITE")
	medianPts = make([]mkt.IndexPoint, 0, len(ref))
	weightedPts = make([]mkt.IndexPoint, 0, len(ref))
	medianCandles = make([]domain.Candle, 0, len(ref))
	weightedCandles = make([]domain.Candle, 0, len(ref))

	var medV, wV float64 = 100, 100
	for i, ts := range ref {
		t := time.Unix(ts, 0).UTC()
		if i == 0 {
			medianPts = append(medianPts, mkt.IndexPoint{Timestamp: ts, Value: 100})
			weightedPts = append(weightedPts, mkt.IndexPoint{Timestamp: ts, Value: 100})
			vol0 := sumVolumeAt(active, ts)
			medianCandles = append(medianCandles, domain.NewCandleUnsafe(
				synthSym, tf, t, 100, 100, 100, 100, vol0,
			))
			weightedCandles = append(weightedCandles, domain.NewCandleUnsafe(
				synthSym, tf, t, 100, 100, 100, 100, vol0,
			))
			continue
		}

		mAgg, mOK := aggregateLogBar(active, ts, false)
		if !mOK {
			continue
		}
		wAgg, wOK := aggregateLogBar(active, ts, true)
		if !wOK {
			wAgg = mAgg
		}

		prevMed, prevW := medV, wV
		medV = prevMed * math.Exp(mAgg.c)
		wV = prevW * math.Exp(wAgg.c)

		mOpen, mHigh, mLow := clampHL(
			prevMed*math.Exp(mAgg.o),
			prevMed*math.Exp(mAgg.h),
			prevMed*math.Exp(mAgg.l),
			medV,
		)
		wOpen, wHigh, wLow := clampHL(
			prevW*math.Exp(wAgg.o),
			prevW*math.Exp(wAgg.h),
			prevW*math.Exp(wAgg.l),
			wV,
		)

		medianPts = append(medianPts, mkt.IndexPoint{Timestamp: ts, Value: medV})
		weightedPts = append(weightedPts, mkt.IndexPoint{Timestamp: ts, Value: wV})
		medianCandles = append(medianCandles, domain.NewCandleUnsafe(
			synthSym, tf, t, mOpen, mHigh, mLow, medV, mAgg.vol,
		))
		weightedCandles = append(weightedCandles, domain.NewCandleUnsafe(
			synthSym, tf, t, wOpen, wHigh, wLow, wV, mAgg.vol,
		))
	}
	return medianPts, weightedPts, medianCandles, weightedCandles
}

// referenceTimeline builds the emit timeline from an already-selected active set:
// print coverage ≥50%, then return-pair coverage ≥50% after the anchor, then
// trim to the last `limit` bars.
func referenceTimeline(active map[string]*symbolBars, limit int) []int64 {
	nSym := len(active)
	if nSym == 0 {
		return nil
	}
	need := (nSym + 1) / 2 // ceil(n/2)

	printFreq := make(map[int64]int)
	for _, p := range active {
		for ts, b := range p.byTS {
			if b.positive() {
				printFreq[ts]++
			}
		}
	}
	candidates := make([]int64, 0, len(printFreq))
	for ts, c := range printFreq {
		if c >= need {
			candidates = append(candidates, ts)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	if len(candidates) == 0 {
		return nil
	}

	out := make([]int64, 0, len(candidates))
	out = append(out, candidates[0]) // anchor — no return pair required
	for _, ts := range candidates[1:] {
		if returnPairCoverage(active, ts) >= need {
			out = append(out, ts)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// activePaths selects symbols that print on the latest dense tip run — not a
// connected component over full last-N history (a bridge bar must not pull in
// a dead cluster). Far-ahead weak islands are stripped per
// stripTrailingWeakIslands. Active = names with a positive print on the latest
// contiguous denseFloor run.
func activePaths(paths map[string]*symbolBars) map[string]*symbolBars {
	n := len(paths)
	if n == 0 {
		return paths
	}
	need := (n + 1) / 2 // ceil(n/2)

	freq := make(map[int64]int)
	for _, p := range paths {
		for ts, b := range p.byTS {
			if b.positive() {
				freq[ts]++
			}
		}
	}
	if len(freq) == 0 {
		return paths
	}
	allTS := make([]int64, 0, len(freq))
	for ts := range freq {
		allTS = append(allTS, ts)
	}
	sort.Slice(allTS, func(i, j int) bool { return allTS[i] < allTS[j] })

	allTS, keptIsland := stripTrailingWeakIslands(paths, allTS, freq, need, n)

	tMin, tMax := allTS[0], allTS[len(allTS)-1]
	cutoff := tMin + 3*(tMax-tMin)/4
	// Minority island that survived keep owns the live clock (weekend / 4-bar
	// holes included). keptIsland is set in one place inside strip — no second
	// keepContinuingIsland call here.
	if keptIsland >= 0 && allTS[keptIsland] > cutoff {
		cutoff = allTS[keptIsland]
	}
	recent := make([]int64, 0, len(allTS)/4+1)
	for _, ts := range allTS {
		if ts >= cutoff {
			recent = append(recent, ts)
		}
	}
	if len(recent) == 0 {
		recent = []int64{tMax}
	}

	maxCRecent := 0
	for _, ts := range recent {
		if freq[ts] > maxCRecent {
			maxCRecent = freq[ts]
		}
	}

	// Floor: when any recent tip clears 50%, use need so a one-bar tip miss
	// still joins the contiguous dense run; otherwise densest recent (minority
	// live vs larger old universe).
	denseFloor := maxCRecent
	hasMajority := false
	for _, ts := range recent {
		if freq[ts] >= need {
			hasMajority = true
			break
		}
	}
	if hasMajority {
		denseFloor = need
	}

	var tip int64
	found := false
	for _, ts := range recent {
		if freq[ts] >= denseFloor && (!found || ts >= tip) {
			tip = ts
			found = true
		}
	}
	if !found {
		return paths
	}

	tipIdx := -1
	for i, ts := range allTS {
		if ts == tip {
			tipIdx = i
			break
		}
	}
	// Run gaps use a robust bar step (ignore island gaps and 1s noise).
	// Allow up to five missing bars; six or more drop the prior side.
	step := robustStep(allTS)
	runGapLimit := int64(runHoleBars) * step
	denseTS := map[int64]struct{}{tip: {}}
	for i := tipIdx - 1; i >= 0; i-- {
		if allTS[i] < cutoff {
			break
		}
		if freq[allTS[i]] < denseFloor {
			break
		}
		if allTS[i+1]-allTS[i] > runGapLimit {
			break
		}
		denseTS[allTS[i]] = struct{}{}
	}

	active := make(map[string]*symbolBars)
	for name, p := range paths {
		for ts := range denseTS {
			if b, ok := p.byTS[ts]; ok && b.positive() {
				active[name] = p
				break
			}
		}
	}
	if len(active) == 0 {
		return paths
	}
	return active
}

// islandGapBars: window of prefix tip stamps counted for continuation overlap.
const islandGapBars = 20

// stubEqualDensityPrefix: equal-density keep (|islandMax| ≥ prefix peak) is
// allowed only for tiny prefixes that fit LatestTip / clean 3-vs-3 — not a
// 19-bar staggered last-N.
const stubEqualDensityPrefix = 8

// resumedLastNBars: minimum island length for a pure no-overlap keep (GetLastN
// after a gap with no tip-window hits on any island printer).
const resumedLastNBars = 100

// runHoleBars: tip-run may bridge this many steps (five missing bars).
// Product policy: six or more missing bars drop the prior side of the run.
const runHoleBars = 6

// minPrefixOverlap: a name counts as continuing only with at least this many
// positive prints on the last islandGapBars stamps of the prefix.
const minPrefixOverlap = 10

// stripTrailingWeakIslands classifies every trailing island as keep or strip.
// keptIsland is the start index of a kept minority island (cutoff bump); -1 if none.
//
//   - Same-cohort tip-miss: islandMax ≥ need and every island printer continues
//     → leave island (no bump; tip-run bridges the hole).
//   - keepContinuingIsland → keep + bump.
//   - Otherwise strip.
func stripTrailingWeakIslands(paths map[string]*symbolBars, allTS []int64, freq map[int64]int, need, n int) ([]int64, int) {
	for len(allTS) > 1 {
		start, ok := trailingIslandStart(allTS)
		if !ok {
			return allTS, -1
		}

		prefix := allTS[:start]
		island := allTS[start:]
		islandMax := 0
		for _, ts := range island {
			if freq[ts] > islandMax {
				islandMax = freq[ts]
			}
		}
		prefixMax := 0
		for _, ts := range prefix {
			if freq[ts] > prefixMax {
				prefixMax = freq[ts]
			}
		}
		onIsland, continuing := islandContinuation(paths, prefix, island)
		majorityIsland := islandMax >= need

		if majorityIsland && onIsland > 0 && continuing == onIsland {
			return allTS, -1
		}

		if keepContinuingIsland(onIsland, continuing, islandMax, n, len(island), len(prefix), prefixMax, majorityIsland) {
			return allTS, start
		}
		allTS = allTS[:start]
	}
	return allTS, -1
}

// trailingIslandStart finds the start of a trailing island separated by more
// than one robust bar step (one missing bar). Sub-step 1s noise stays in-island.
func trailingIslandStart(allTS []int64) (int, bool) {
	if len(allTS) < 2 {
		return 0, false
	}
	step := robustStep(allTS)
	if step <= 0 {
		step = 1
	}
	start := len(allTS) - 1
	for start > 0 && allTS[start]-allTS[start-1] <= step {
		start--
	}
	if start == 0 {
		return 0, false
	}
	return start, true
}

// islandContinuation counts island printers and how many have ≥ minPrefixOverlap
// hits on the last islandGapBars stamps of the prefix.
func islandContinuation(paths map[string]*symbolBars, prefix, island []int64) (onIsland, continuing int) {
	tipPrefix := prefix
	if len(prefix) > islandGapBars {
		tipPrefix = prefix[len(prefix)-islandGapBars:]
	}
	tipSet := make(map[int64]struct{}, len(tipPrefix))
	for _, ts := range tipPrefix {
		tipSet[ts] = struct{}{}
	}
	islandSet := make(map[int64]struct{}, len(island))
	for _, ts := range island {
		islandSet[ts] = struct{}{}
	}
	for _, p := range paths {
		hasIsland := false
		tipHits := 0
		for ts, b := range p.byTS {
			if !b.positive() {
				continue
			}
			if _, ok := islandSet[ts]; ok {
				hasIsland = true
			}
			if _, ok := tipSet[ts]; ok {
				tipHits++
			}
		}
		if hasIsland {
			onIsland++
			if tipHits >= minPrefixOverlap {
				continuing++
			}
		}
	}
	return onIsland, continuing
}

// keepContinuingIsland is true when a trailing island should survive strip and
// own the recent-window cutoff. Requires islandMax ≥ n/3 and:
//
//   - every island printer tip-continues, or
//   - |island| ≥ resumedLastNBars with no tip-window hits (pure resume), or
//   - |prefix| < stubEqualDensityPrefix, islandMax ≤ 3, and islandMax ≥ densest
//     prefix tip (LatestTip / clean 3-vs-3 only — not stub 5-vs-5).
//
// A majority island that is not tip-continuation and not a stub equal-density
// only length-keeps — 5-vs-5 listings strip even on short prefixes.
func keepContinuingIsland(onIsland, continuing, islandMax, n, islandLen, prefixLen, prefixMax int, majorityIsland bool) bool {
	if n <= 0 || onIsland == 0 || islandMax*3 < n {
		return false
	}
	if continuing > 0 {
		return continuing == onIsland
	}
	if islandLen >= resumedLastNBars {
		return true
	}
	// Cap at 3 so stub 5-vs-5 / 5-new cannot keep via islandMax ≥ prefixMax.
	if prefixLen < stubEqualDensityPrefix && islandMax <= 3 && islandMax >= prefixMax {
		return true
	}
	if majorityIsland {
		return false
	}
	return false
}

// robustStep is the minimum gap among gaps in [median/2, 2×median], ignoring
// far island gaps and one-off sub-step noise (e.g. 1s misaligned prints).
func robustStep(allTS []int64) int64 {
	if len(allTS) < 2 {
		return 1
	}
	med := medianStep(allTS)
	if med <= 0 {
		med = 1
	}
	lo := med / 2
	if lo < 1 {
		lo = 1
	}
	hi := 2 * med
	step := int64(0)
	for i := 1; i < len(allTS); i++ {
		g := allTS[i] - allTS[i-1]
		if g >= lo && g <= hi {
			if step == 0 || g < step {
				step = g
			}
		}
	}
	if step <= 0 {
		return med
	}
	return step
}

func medianStep(allTS []int64) int64 {
	if len(allTS) < 2 {
		return 1
	}
	gaps := make([]int64, 0, len(allTS)-1)
	for i := 1; i < len(allTS); i++ {
		gaps = append(gaps, allTS[i]-allTS[i-1])
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	return gaps[len(gaps)/2]
}

func returnPairCoverage(paths map[string]*symbolBars, ts int64) int {
	n := 0
	for _, p := range paths {
		cur, ok := p.byTS[ts]
		if !ok || !cur.positive() {
			continue
		}
		if _, okPrev := p.closeBefore(ts); okPrev {
			n++
		}
	}
	return n
}

type logAgg struct {
	o, h, l, c, vol float64
}

// aggregateLogBar aggregates ln(price / lastCloseBefore(ts)) for symbols
// that print at ts with a prior positive close. ok is false when no pairs.
func aggregateLogBar(
	paths map[string]*symbolBars,
	ts int64,
	weighted bool,
) (logAgg, bool) {
	var opens, highs, lows, closes []float64
	var wOpen, wHigh, wLow, wClose, wSum, vol float64
	for _, p := range paths {
		cur, okCur := p.byTS[ts]
		if !okCur || !cur.positive() {
			continue
		}
		prevClose, okPrev := p.closeBefore(ts)
		if !okPrev {
			continue
		}
		vol += cur.volume
		ro := math.Log(cur.open / prevClose)
		rh := math.Log(cur.high / prevClose)
		rl := math.Log(cur.low / prevClose)
		rc := math.Log(cur.close / prevClose)
		if weighted {
			w := p.weight
			wSum += w
			wOpen += ro * w
			wHigh += rh * w
			wLow += rl * w
			wClose += rc * w
		} else {
			opens = append(opens, ro)
			highs = append(highs, rh)
			lows = append(lows, rl)
			closes = append(closes, rc)
		}
	}
	if weighted {
		if wSum <= 0 {
			return logAgg{}, false
		}
		return logAgg{
			o: wOpen / wSum, h: wHigh / wSum, l: wLow / wSum, c: wClose / wSum, vol: vol,
		}, true
	}
	if len(closes) == 0 {
		return logAgg{}, false
	}
	return logAgg{
		o: median(opens), h: median(highs), l: median(lows), c: median(closes), vol: vol,
	}, true
}

func sumVolumeAt(paths map[string]*symbolBars, ts int64) float64 {
	var v float64
	for _, p := range paths {
		if b, ok := p.byTS[ts]; ok && b.positive() {
			v += b.volume
		}
	}
	return v
}

func clampHL(o, h, l, c float64) (float64, float64, float64) {
	if h < c {
		h = c
	}
	if h < o {
		h = o
	}
	if l > c {
		l = c
	}
	if l > o {
		l = o
	}
	return o, h, l
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}
