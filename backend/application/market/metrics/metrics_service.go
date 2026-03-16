package metrics

import (
	"context"
	"sync"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// EvaluationProvider supplies per-symbol evaluation snapshots.
type EvaluationProvider interface {
	GetLatestEvaluations(timeframe string) ([]domain.EvaluationSnapshot, error)
}

// MetricsService is the central entry point for all market-level analytics.
// It shares candle and evaluation data across metrics to avoid duplicate queries.
type MetricsService struct {
	compositeService *CompositeIndexService
	candleProvider   CandleProvider
	evalProvider     EvaluationProvider
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

// CalculateRegime computes the full regime summary for a timeframe.
// It fetches candles and evaluations once, then derives all metrics.
func (s *MetricsService) CalculateRegime(ctx context.Context, timeframe string) (mkt.RegimeSummary, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return mkt.RegimeSummary{}, err
	}

	// Fetch evaluations for breadth.
	evals, err := s.evalProvider.GetLatestEvaluations(timeframe)
	if err != nil {
		return mkt.RegimeSummary{}, err
	}

	trendBreadth, compressionBreadth := breadthFromEvaluations(evals)

	// Fetch candles for volatility and dispersion.
	symbols, err := s.candleProvider.Symbols(ctx)
	if err != nil {
		return mkt.RegimeSummary{}, err
	}

	type candleResult struct {
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

			cs, fetchErr := s.candleProvider.GetLastNCandles(sym, tf, 30)
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
			results = append(results, candleResult{candles: candles, ret: ret})
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

	metrics := mkt.RegimeMetrics{
		TrendBreadth:        trendBreadth,
		CompressionBreadth:  compressionBreadth,
		VolatilityExpansion: volExpansion,
		Dispersion:          disp,
	}

	regime, confidence := detectRegime(trendBreadth, compressionBreadth, volExpansion)

	return mkt.RegimeSummary{
		Timeframe:  timeframe,
		Regime:     regime,
		Confidence: confidence,
		Metrics:    metrics,
	}, nil
}

// breadthFromEvaluations computes the fraction of symbols classified as
// trend or compression from their evaluation scores.
func breadthFromEvaluations(evals []domain.EvaluationSnapshot) (trendBreadth, compressionBreadth float64) {
	if len(evals) == 0 {
		return 0, 0
	}

	trendCount := 0
	compressionCount := 0

	for _, e := range evals {
		if e.TrendScore > 0.65 {
			trendCount++
		}
		if e.CompressionScore > 0.7 {
			compressionCount++
		}
	}

	total := float64(len(evals))
	return float64(trendCount) / total, float64(compressionCount) / total
}
