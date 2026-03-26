package metrics

import (
	"context"
	"math"
	"sync"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// EvaluationProvider supplies per-symbol evaluation snapshots.
// It is retained in the constructor signature for backwards compatibility
// but is no longer used by CalculateRegime; breadth is now derived directly
// from candle data via proxyBreadth.
type EvaluationProvider interface {
	GetLatestEvaluations(timeframe string) ([]domain.EvaluationSnapshot, error)
}

// RegimeObserver is notified after every regime calculation.
// The Tracker from the regimehistory package satisfies this interface.
type RegimeObserver interface {
	Update(timeframe string, regime mkt.Regime, timestamp int64) error
}

// MetricsService is the central entry point for all market-level analytics.
// It shares candle and evaluation data across metrics to avoid duplicate queries.
type MetricsService struct {
	compositeService *CompositeIndexService
	candleProvider   CandleProvider
	evalProvider     EvaluationProvider
	observer         RegimeObserver // optional — may be nil
}

// NewMetricsService constructs the aggregator.
func NewMetricsService(
	c *CompositeIndexService,
	cp CandleProvider,
	ep EvaluationProvider,
) *MetricsService {
	return &MetricsService{
		compositeService: c,
		candleProvider:   cp,
		evalProvider:     ep,
	}
}

// SetObserver attaches a regime observer (e.g. the history tracker).
func (s *MetricsService) SetObserver(o RegimeObserver) {
	s.observer = o
}

// CalculateRegime computes the full regime summary for a timeframe.
// It fetches candles and evaluations once, then derives all metrics.
func (s *MetricsService) CalculateRegime(ctx context.Context, timeframe string) (mkt.RegimeSummary, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return mkt.RegimeSummary{}, err
	}

	// Fetch candles for all metrics (volatility, dispersion, breadth).
	symbols, err := s.candleProvider.Symbols(ctx)
	if err != nil {
		return mkt.RegimeSummary{}, err
	}

	type candleResult struct {
		series  domain.CandleSeries // kept for real scoring
		candles []domain.Candle
		ret     float64 // period return
	}

	var mu sync.Mutex
	var results []candleResult
	sem := make(chan struct{}, 20) // bounded parallelism
	var wg sync.WaitGroup

	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cs, fetchErr := s.candleProvider.GetLastNCandles(sym, tf, metricsWindow)
			if fetchErr != nil || cs.Len() < 2 {
				return
			}
			candles := cs.All()
			first := candles[0].Close()
			last := candles[len(candles)-1].Close()
			ret := 0.0
			if first != 0 {
				ret = (last - first) / first
			}

			mu.Lock()
			results = append(results, candleResult{series: cs, candles: candles, ret: ret})
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Compute volatility expansion — median across all symbols.
	volExpansion := 1.0
	if len(results) > 0 {
		volValues := make([]float64, len(results))
		for i, r := range results {
			volValues[i] = volatilityExpansion(r.candles)
		}
		volExpansion = median(volValues)
	}

	// Compute dispersion — MAD of asset returns vs market return.
	disp := 0.0
	if len(results) > 0 {
		returns := make([]float64, len(results))
		sum := 0.0
		for i, r := range results {
			returns[i] = r.ret
			sum += r.ret
		}
		marketReturn := sum / float64(len(returns))
		disp = dispersion(returns, marketReturn)
	}

	// Compute breadth using the real domain/scoring calculators —
	// the same algorithms used by the overview/rankings pipeline.
	seriesList := make([]domain.CandleSeries, 0, len(results))
	for _, r := range results {
		if r.series.Len() >= 2 {
			seriesList = append(seriesList, r.series)
		}
	}
	breadth := scoreBreadth(seriesList, timeframe)

	// Derive expansion breadth: amplify raw breakout average with volatility
	// so the exposed metric reflects the same "expansion" concept used in
	// regime scoring (breakout activity + elevated volatility).
	expansionBreadth := math.Min(breadth.expansion*(1+math.Max(0, volExpansion-1.0)*1.5), 1.0)

	metrics := mkt.RegimeMetrics{
		TrendBreadth:        breadth.trend,
		SidewaysBreadth:     breadth.sideways,
		CompressionBreadth:  breadth.compression,
		ExpansionBreadth:    expansionBreadth,
		VolatilityExpansion: volExpansion,
		Dispersion:          disp,
	}

	// --- Health dampening (backported from MarketStateService) ---
	// Fetch evaluation snapshots for trend health and silent detection.
	var effectiveTrend, breakdownRate float64
	evaluations, evalErr := s.evalProvider.GetLatestEvaluations(timeframe)
	hasHealthData := false

	if evalErr == nil && len(evaluations) > 0 {
		var healthyCount, effectiveSum, breakdowns float64
		for _, e := range evaluations {
			if e.ATR == 0 {
				continue
			}
			// Only consider tokens where trend is the dominant regime.
			breakoutMax := math.Max(e.BreakoutUpScore, e.BreakoutDownScore)
			ts := math.Abs(e.TrendScore)
			if ts < e.SidewaysScore || ts < e.CompressionScore || ts < breakoutMax {
				continue
			}
			healthyCount++
			state := "uptrend"
			if e.Bias == "down" {
				state = "downtrend"
			}
			h := appmarket.ComputeTrendHealth(state, e.Price, e.RecentHigh, e.RecentLow, e.ATR, e.RecentReturn)
			effectiveSum += h
			if h < 0.4 {
				breakdowns++
			}
		}
		if healthyCount > 0 {
			hasHealthData = true
			effectiveTrend = effectiveSum / float64(len(evaluations))
			breakdownRate = breakdowns / healthyCount

			// Dampen trend breadth when health is poor.
			dampBreadth := mkt.Breadth{
				Trend:       breadth.trend,
				Sideways:    breadth.sideways,
				Compression: breadth.compression,
				Expansion:   breadth.expansion,
			}
			dampBreadth = appmarket.DampenTrendByHealth(dampBreadth, effectiveTrend, breakdownRate)
			breadth.trend = dampBreadth.Trend
			breadth.sideways = dampBreadth.Sideways
			breadth.compression = dampBreadth.Compression
			breadth.expansion = dampBreadth.Expansion
		}
	}

	regime, prevalence, scores := detectRegime(breadth, volExpansion)

	// --- Indecisive guard ---
	regime, prevalence = applyIndecisiveGuard(regime, scores)

	// --- Silent override ---
	// Only when dominant regime is sideways or indecisive, returns are flat,
	// and volume is not elevated.
	if (regime == mkt.RegimeSideways || regime == mkt.RegimeIndecisive) && evalErr == nil && len(evaluations) > 0 {
		avgAbsReturn, avgVolume, medianVolume := appmarket.AggregateActivityMetrics(evaluations)
		hasVolumeData := medianVolume > 0
		volumeNormal := hasVolumeData && avgVolume <= medianVolume*1.5
		if hasVolumeData && avgAbsReturn < 0.5 && volumeNormal {
			regime = mkt.RegimeSilent
		}
	}

	label := appmarket.BuildMarketLabel(breadth.trend, effectiveTrend)
	_ = hasHealthData

	// Compute directional bias from aggregate returns.
	// Hard gate: if aggregate return is negative, bias cannot be "up".
	// If positive, cannot be "down".  The return sign is the truth.
	bias := "neutral"
	if len(results) > 0 {
		var returnSum float64
		for _, r := range results {
			returnSum += r.ret
		}
		avgReturn := returnSum / float64(len(results))
		if avgReturn > 0 {
			bias = "up"
		} else if avgReturn < 0 {
			bias = "down"
		}
	}

	// Notify observer (e.g. regime history tracker) — fire-and-forget.
	if s.observer != nil {
		_ = s.observer.Update(timeframe, regime, time.Now().Unix())
	}

	return mkt.RegimeSummary{
		Timeframe:      timeframe,
		Regime:         regime,
		Prevalence:     prevalence,
		Scores:         scores,
		Metrics:        metrics,
		Bias:           bias,
		EffectiveTrend: effectiveTrend,
		BreakdownRate:  breakdownRate,
		Label:          label,
	}, nil
}
