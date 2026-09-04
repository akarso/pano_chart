package metrics

import (
	"context"
	"sort"
	"sync"

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

// CompositeIndexService computes a normalized composite market index.
// For each symbol the close prices are rebased to 100 at the first candle,
// then the median across all symbols is taken at each timestamp.
type CompositeIndexService struct {
	provider    CandleProvider
	workerLimit int
}

// NewCompositeIndexService constructs the service.
func NewCompositeIndexService(p CandleProvider, workerLimit int) *CompositeIndexService {
	if workerLimit <= 0 {
		workerLimit = 20
	}
	return &CompositeIndexService{provider: p, workerLimit: workerLimit}
}

// Calculate produces a composite index for the given timeframe with at most
// `limit` data points.
func (s *CompositeIndexService) Calculate(ctx context.Context, timeframe string, limit int) (mkt.CompositeIndex, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return mkt.CompositeIndex{}, err
	}
	if limit <= 0 {
		limit = 200
	}

	symbols, err := s.provider.Symbols(ctx)
	if err != nil {
		return mkt.CompositeIndex{}, err
	}
	if len(symbols) == 0 {
		return mkt.CompositeIndex{Timeframe: timeframe, SymbolCount: 0}, nil
	}

	// Fetch candles concurrently with bounded parallelism.
	type result struct {
		series []float64 // normalised to base 100
		stamps []int64   // unix timestamps
	}

	var mu sync.Mutex
	var results []result

	sem := make(chan struct{}, s.workerLimit)
	var wg sync.WaitGroup

	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cs, fetchErr := s.provider.GetLastNCandles(ctx, sym, tf, limit)
			if fetchErr != nil || cs.Len() < 2 {
				return // skip symbols with errors or insufficient data
			}

			candles := cs.All()
			base := candles[0].Close()
			if base == 0 {
				return // avoid division by zero
			}

			normed := make([]float64, len(candles))
			stamps := make([]int64, len(candles))
			for i, c := range candles {
				normed[i] = (c.Close() / base) * 100
				stamps[i] = c.Timestamp().Unix()
			}

			mu.Lock()
			results = append(results, result{series: normed, stamps: stamps})
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(results) == 0 {
		return mkt.CompositeIndex{Timeframe: timeframe, SymbolCount: len(symbols)}, nil
	}

	// Use timestamps from the longest series as reference.
	ref := results[0]
	for _, r := range results[1:] {
		if len(r.stamps) > len(ref.stamps) {
			ref = r
		}
	}

	points := make([]mkt.IndexPoint, 0, len(ref.stamps))
	for i, ts := range ref.stamps {
		var values []float64
		for _, r := range results {
			if i < len(r.series) {
				values = append(values, r.series[i])
			}
		}
		if len(values) == 0 {
			continue
		}
		points = append(points, mkt.IndexPoint{
			Timestamp: ts,
			Value:     median(values),
		})
	}

	return mkt.CompositeIndex{
		Timeframe:   timeframe,
		Points:      points,
		SymbolCount: len(results),
	}, nil
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
