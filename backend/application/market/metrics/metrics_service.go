package metrics

import (
	"context"
	"sync"
	"time"

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

	metrics := mkt.RegimeMetrics{
		TrendBreadth:        breadth.trend,
		SidewaysBreadth:     breadth.sideways,
		CompressionBreadth:  breadth.compression,
		BreakoutBreadth:     breadth.breakout,
		VolatilityExpansion: volExpansion,
		Dispersion:          disp,
	}

	regime, prevalence, scores := detectRegime(breadth, volExpansion)

	// Notify observer (e.g. regime history tracker) — fire-and-forget.
	if s.observer != nil {
		_ = s.observer.Update(timeframe, regime, time.Now().Unix())
	}

	return mkt.RegimeSummary{
		Timeframe:  timeframe,
		Regime:     regime,
		Prevalence: prevalence,
		Scores:     scores,
		Metrics:    metrics,
	}, nil
}
