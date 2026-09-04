package market

import (
	"context"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// CandleProvider provides candle data and symbol lists for market metrics.
// Optional dependency of MarketStateService — see SetCandleProvider.
type CandleProvider interface {
	// Symbols returns the current symbol universe.
	Symbols(ctx context.Context) ([]domain.Symbol, error)
	// GetLastNCandles retrieves the last N candles for a symbol and timeframe.
	GetLastNCandles(ctx context.Context, symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error)
}

// RegimeObserver is notified after every Calculate call. The Tracker from
// the regimehistory package satisfies this interface.
type RegimeObserver interface {
	Update(timeframe string, regime mkt.Regime, timestamp int64) error
}

// candleMetricsWindow is the candle window used for VolatilityExpansion /
// Dispersion — matches the sparkline precision used elsewhere.
const candleMetricsWindow = 110

// candleMetricsFanoutLimit bounds concurrent candle fetches in
// candleMetrics — one goroutine per symbol would be excessive for a large
// universe.
const candleMetricsFanoutLimit = 20

// expectedSymbolCount is the usual symbol-universe size, used to flag
// DataQuality as degraded when meaningfully fewer evaluations came in than
// normal (partial fetch failures, a struggling upstream) — see PR-074. A
// starting point, not a permanent guess: tune from real outage telemetry.
const expectedSymbolCount = 150

// MarketStateService computes the aggregate market state summary
// by classifying each symbol's evaluation snapshot and computing
// breadth ratios.
type MarketStateService struct {
	provider EvaluationProvider
	candles  CandleProvider // optional; nil disables VolatilityExpansion/Dispersion
	observer RegimeObserver // optional; nil disables history tracking
}

// NewMarketStateService constructs the service.
func NewMarketStateService(p EvaluationProvider) *MarketStateService {
	return &MarketStateService{provider: p}
}

// SetCandleProvider enables VolatilityExpansion/Dispersion computation.
// Without it, VolatilityExpansion defaults to 1.0 ("normal") and Dispersion
// to 0 — Calculate remains fully usable, just without these two metrics.
func (s *MarketStateService) SetCandleProvider(cp CandleProvider) {
	s.candles = cp
}

// SetObserver attaches a regime observer (e.g. the history tracker).
func (s *MarketStateService) SetObserver(o RegimeObserver) {
	s.observer = o
}

// Calculate produces a market state summary for the given timeframe from
// evaluation snapshots only — no candle I/O, cheap regardless of symbol
// universe size. VolatilityExpansion/Dispersion are left at their defaults
// (1.0/0); use CalculateWithCandleMetrics when a caller actually needs them
// (the legacy /api/market/regime response, the transition engine's
// volatility slope). Consumers that don't read those two fields —
// /api/market/state, the notification scheduler, the setup scanner — should
// keep using this cheaper method: see PR-073 CR follow-up, which found
// every Calculate() call was paying for a full symbol-universe candle
// fan-out regardless of whether the caller needed it.
//
// Breadth is computed using proportional weighting: every symbol distributes
// its scores continuously across all four regimes (sideways, compression,
// expansion, trend).  This eliminates the zero-breadth problem that occurred
// with binary classification thresholds.
//
// Trend prevalence is health-dampened: if most "trending" tokens are actually
// falling apart (price far from extremes, large drawdowns), the trend breadth
// is penalised before state determination.  This prevents a broken market
// from being classified as "Trend 94%" just because individual tokens have
// moderate R² values that happen to exceed their other scores.
func (s *MarketStateService) Calculate(timeframe string) (mkt.Summary, error) {
	evaluations, err := s.provider.GetLatestEvaluations(timeframe)
	if err != nil {
		return mkt.Summary{}, err
	}

	if len(evaluations) == 0 {
		// No observer notification here (intentional): a "no data" period
		// isn't a regime observation, so it isn't recorded as one. Regime
		// history will show a gap rather than a fabricated "sideways" point
		// for whatever window had no evaluations.
		return mkt.Summary{
			Timeframe:           timeframe,
			State:               mkt.StateSideways,
			Confidence:          0,
			Breadth:             mkt.Breadth{},
			SymbolCount:         0,
			VolatilityExpansion: 1.0,
			Label:               BuildMarketLabel(0, 0),
			DataQuality:         mkt.DataQualityUnavailable,
		}, nil
	}

	// Static threshold, not a new EvaluationProvider method — EvaluationProvider
	// (application/market/evaluation_provider.go) has exactly one method today
	// (GetLatestEvaluations); adding a second would mean implementing it on
	// every real provider AND every test fake for a number that's really just
	// "the usual universe size", which doesn't change per-call. Tune from real
	// outage telemetry, not kept as a guess forever — see PR-074.
	dataQuality := mkt.DataQualityOK
	// 2*n < expected, not n < expected/2 — integer division floors, so with
	// an odd expectedSymbolCount the /2 form under-flags right at the
	// boundary (e.g. expected=151: expected/2==75, so n==75 reads as "not
	// degraded" even though 75 is less than half of 151).
	if 2*len(evaluations) < expectedSymbolCount {
		dataQuality = mkt.DataQualityDegraded
	}
	// Logged (timeframe-tagged) so expectedSymbolCount can actually be
	// tuned from observed counts rather than staying a permanent guess —
	// see PR-074 CR follow-up. Only on non-OK, to avoid a log line on every
	// single Calculate() call in steady state.
	if dataQuality != mkt.DataQualityOK {
		log.Printf("[market] %s: %d evaluations (expected ~%d) — DataQuality=%s", timeframe, len(evaluations), expectedSymbolCount, dataQuality)
	}

	total := float64(len(evaluations))

	// ---- 1. Raw proportional breadth ----
	var breadth mkt.Breadth
	for _, e := range evaluations {
		w := scoreWeights(e)
		breadth.Sideways += w.Sideways
		breadth.Compression += w.Compression
		breadth.Expansion += w.Expansion
		breadth.Trend += w.Trend
	}
	breadth.Sideways /= total
	breadth.Compression /= total
	breadth.Expansion /= total
	breadth.Trend /= total

	// ---- 2. Trend health (compute BEFORE state determination) ----
	var trendCount, healthyTrendCount, effectiveSum, breakdowns float64
	for _, e := range evaluations {
		w := scoreWeights(e)
		if w.Trend < w.Sideways || w.Trend < w.Compression || w.Trend < w.Expansion {
			continue
		}
		trendCount++
		// Skip tokens without price data — they can't contribute to health.
		if e.ATR == 0 {
			continue
		}
		healthyTrendCount++
		state := "uptrend"
		if e.Bias == "down" {
			state = "downtrend"
		}
		h := ComputeTrendHealth(state, e.Price, e.RecentHigh, e.RecentLow, e.ATR, e.RecentReturn)
		effectiveSum += h
		if h < 0.4 {
			breakdowns++
		}
	}

	var effectiveTrend, breakdownRate float64
	if total > 0 {
		effectiveTrend = effectiveSum / total
	}
	if healthyTrendCount > 0 {
		breakdownRate = breakdowns / healthyTrendCount
	}

	// ---- 3. Health-dampen trend prevalence ----
	// Only apply when we have meaningful health data from tokens with
	// price information.  Without ATR/price data, dampening would
	// incorrectly penalize trends.
	if healthyTrendCount > 0 {
		breadth = DampenTrendByHealth(breadth, effectiveTrend, breakdownRate)
	}

	// ---- 4. Dominant state from health-adjusted breadth ----
	dominant := mkt.StateSideways
	maxWeight := breadth.Sideways

	if breadth.Trend >= maxWeight {
		dominant = mkt.StateTrend
		maxWeight = breadth.Trend
	}
	if breadth.Compression >= maxWeight {
		dominant = mkt.StateCompression
		maxWeight = breadth.Compression
	}
	if breadth.Expansion >= maxWeight {
		dominant = mkt.StateExpansion
		maxWeight = breadth.Expansion
	}

	// Second-highest breadth for indecisive check.
	weights := []float64{breadth.Sideways, breadth.Compression,
		breadth.Expansion, breadth.Trend}
	first, second := topTwo(weights)

	// ---- 4a. Indecisive override ----
	// Rule 1: no regime above 50% → indecisive.
	// Rule 2: gap between top two < 30pp → indecisive.
	if first < 0.50 || (first-second) < 0.30 {
		dominant = mkt.StateIndecisive
		maxWeight = first
	}

	// ---- 4b. Silent override (flat activity, low volume) ----
	// Silent = sideways-dominant or indecisive, near-zero absolute
	// return, and volume not elevated.  A flat chart with high volume
	// signals something cooking, not silence.
	// We only apply the silent override when volume data is present —
	// without it we can't distinguish silence from missing data.
	if dominant == mkt.StateSideways || dominant == mkt.StateIndecisive {
		avgAbsReturn, avgVolume, medianVolume := AggregateActivityMetrics(evaluations)
		hasVolumeData := medianVolume > 0
		volumeNormal := hasVolumeData && avgVolume <= medianVolume*1.5
		if hasVolumeData && avgAbsReturn < 0.5 && volumeNormal {
			dominant = mkt.StateSilent
		}
	}

	// ---- 5. Directional bias with aggregate return validation ----
	var upWeight, downWeight float64
	var returnSum float64
	for _, e := range evaluations {
		ts := math.Abs(e.TrendScore)
		switch e.Bias {
		case "up":
			upWeight += ts
		case "down":
			downWeight += ts
		}
		returnSum += e.RecentReturn
	}
	avgReturn := returnSum / total

	bias := "neutral"
	if upWeight > downWeight {
		bias = "up"
	} else if downWeight > upWeight {
		bias = "down"
	}

	// Override: if aggregate return clearly contradicts the bias, demote.
	// A -1.66% aggregate with bias="up" makes no sense.
	if bias == "up" && avgReturn < -0.5 {
		bias = "neutral"
	} else if bias == "down" && avgReturn > 0.5 {
		bias = "neutral"
	}

	trendPrevalence := breadth.Trend
	label := BuildMarketLabel(trendPrevalence, effectiveTrend)

	// Notify observer (e.g. regime history tracker) — fire-and-forget, but
	// logged: a persistent write failure to the regime-history DB would
	// otherwise be invisible.
	if s.observer != nil {
		if err := s.observer.Update(timeframe, mkt.Regime(dominant), time.Now().Unix()); err != nil {
			log.Printf("[market] regime observer update failed for %s: %v", timeframe, err)
		}
	}

	return mkt.Summary{
		Timeframe:           timeframe,
		State:               dominant,
		Confidence:          maxWeight,
		Breadth:             breadth,
		SymbolCount:         len(evaluations),
		Bias:                bias,
		EffectiveTrend:      effectiveTrend,
		BreakdownRate:       breakdownRate,
		Label:               label,
		VolatilityExpansion: 1.0,
		DataQuality:         dataQuality,
	}, nil
}

// CalculateWithCandleMetrics is Calculate plus VolatilityExpansion/Dispersion,
// computed from raw candle data across the whole symbol universe (bounded
// concurrent fetch, candleMetricsFanoutLimit at a time). Meaningfully more
// expensive than Calculate — use it only where these two fields are actually
// consumed. ctx bounds the candle fan-out only; Calculate's own evaluation
// fetch is unchanged (it never took a context, matching its pre-existing
// EvaluationProvider interface).
func (s *MarketStateService) CalculateWithCandleMetrics(ctx context.Context, timeframe string) (mkt.Summary, error) {
	summary, err := s.Calculate(timeframe)
	if err != nil {
		return summary, err
	}
	summary.VolatilityExpansion, summary.Dispersion = s.candleMetrics(ctx, timeframe)
	return summary, nil
}

// candleMetrics computes VolatilityExpansion (median short/long ATR ratio)
// and Dispersion (MAD of period returns vs. the mean market return) across
// the symbol universe. Returns the defaults (1.0, 0) when no CandleProvider
// is configured or the fetch fails — these are supplementary metrics, not
// required for state classification, so a failure here must not fail the
// caller as a whole. Symbols()/timeframe-parse failures are logged rather
// than swallowed silently, so a broken CandleProvider is diagnosable
// instead of reporting VolatilityExpansion=1.0 forever with no signal.
func (s *MarketStateService) candleMetrics(ctx context.Context, timeframe string) (volExpansion, disp float64) {
	volExpansion = 1.0
	if s.candles == nil {
		return volExpansion, 0
	}

	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		log.Printf("[market] candleMetrics: invalid timeframe %q: %v", timeframe, err)
		return volExpansion, 0
	}
	symbols, err := s.candles.Symbols(ctx)
	if err != nil {
		log.Printf("[market] candleMetrics: fetching symbols failed: %v", err)
		return volExpansion, 0
	}
	if len(symbols) == 0 {
		return volExpansion, 0
	}

	results, cancelled := fetchCandleResults(ctx, s.candles, tf, symbols)
	if cancelled {
		log.Printf("[market] candleMetrics: cancelled for %s before all fetches completed; any still in flight will finish in the background and be discarded", timeframe)
	}
	return aggregateCandleResults(results)
}

// candleResult is one symbol's contribution to the market-wide metrics.
type candleResult struct {
	vol float64 // this symbol's volatilityExpansion
	ret float64 // this symbol's period return
}

// fetchCandleResults fetches candleMetricsWindow candles per symbol, bounded
// to candleMetricsFanoutLimit concurrent requests, and computes each
// symbol's volatility/return contribution. A symbol whose fetch errors or
// returns too little data is silently skipped — a single bad symbol
// shouldn't fail the whole market-wide computation.
//
// cancelled reports whether ctx was done before every fetch finished.
// GetLastNCandles now takes ctx and well-behaved implementations (e.g.
// FreeTierCandleRepository, via its underlying HTTP request) abort their
// in-flight I/O when it's cancelled — but a fetch already past the
// concurrency gate is not guaranteed to return before ctx.Done() fires
// (e.g. a provider that ignores ctx, or a cancellation that races the
// response), so this function still races completion against ctx.Done()
// rather than assuming the former always wins. Any such fetch keeps
// running in the background and its result is discarded, so this method
// can return to the caller promptly instead of blocking through the
// straggler's rate-limit waits, retries, or network round-trip.
func fetchCandleResults(ctx context.Context, candles CandleProvider, tf domain.Timeframe, symbols []domain.Symbol) (results []candleResult, cancelled bool) {
	var mu sync.Mutex
	sem := make(chan struct{}, candleMetricsFanoutLimit)
	var wg sync.WaitGroup

	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			cs, fetchErr := candles.GetLastNCandles(ctx, sym, tf, candleMetricsWindow)
			if fetchErr != nil || cs.Len() < 2 {
				return
			}
			all := cs.All()
			first := all[0].Close()
			last := all[len(all)-1].Close()
			ret := 0.0
			if first != 0 {
				ret = (last - first) / first
			}

			mu.Lock()
			results = append(results, candleResult{vol: volatilityExpansion(all), ret: ret})
			mu.Unlock()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return results, false
	case <-ctx.Done():
		return nil, true
	}
}

// aggregateCandleResults reduces per-symbol results to the two market-wide
// metrics: VolatilityExpansion (median across symbols) and Dispersion (MAD
// of returns from the mean return). Pure arithmetic — no I/O, no
// concurrency — so it's independently testable from the fetch/orchestration
// above.
func aggregateCandleResults(results []candleResult) (volExpansion, disp float64) {
	volExpansion = 1.0
	if len(results) == 0 {
		return volExpansion, 0
	}

	vols := make([]float64, len(results))
	returns := make([]float64, len(results))
	var returnSum float64
	for i, r := range results {
		vols[i] = r.vol
		returns[i] = r.ret
		returnSum += r.ret
	}
	volExpansion = median(vols)
	disp = dispersion(returns, returnSum/float64(len(returns)))
	return volExpansion, disp
}

// median returns the median of a slice. Modifies the input in place via sort.
func median(vals []float64) float64 {
	sort.Float64s(vals)
	n := len(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// topTwo returns the two highest values from a slice.
func topTwo(vals []float64) (first, second float64) {
	for _, v := range vals {
		if v >= first {
			second = first
			first = v
		} else if v > second {
			second = v
		}
	}
	return
}

// AggregateActivityMetrics computes the average absolute return (in ATR
// units) and the average + median volume across all evaluations.
// Tokens with ATR == 0 are skipped for the return calculation.
func AggregateActivityMetrics(evals []domain.EvaluationSnapshot) (avgAbsReturn, avgVolume, medianVolume float64) {
	if len(evals) == 0 {
		return
	}

	var absReturnSum float64
	var returnCount float64
	var volSum float64
	volumes := make([]float64, 0, len(evals))

	for _, e := range evals {
		if e.ATR > 0 {
			absReturnSum += math.Abs(e.RecentReturn)
			returnCount++
		}
		if e.Volume > 0 {
			volSum += e.Volume
			volumes = append(volumes, e.Volume)
		}
	}

	if returnCount > 0 {
		avgAbsReturn = absReturnSum / returnCount
	}
	if len(volumes) > 0 {
		avgVolume = volSum / float64(len(volumes))
		// Simple median via insertion sort (≤200 items).
		for i := 1; i < len(volumes); i++ {
			key := volumes[i]
			j := i - 1
			for j >= 0 && volumes[j] > key {
				volumes[j+1] = volumes[j]
				j--
			}
			volumes[j+1] = key
		}
		mid := len(volumes) / 2
		if len(volumes)%2 == 0 {
			medianVolume = (volumes[mid-1] + volumes[mid]) / 2
		} else {
			medianVolume = volumes[mid]
		}
	}
	return
}
