package metrics

import (
	"context"
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

// CompositeIndexService computes a normalized composite market index.
// For each symbol the OHLC prices are rebased to 100 at the first candle,
// then two aggregates are taken at each timestamp: equal-weight median and
// quote-volume-weighted mean (see PR-084).
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
		return CompositeTape{
			Index:           mkt.CompositeIndex{Timeframe: timeframe, SymbolCount: 0},
			PreferredSource: "composite_median",
		}, nil
	}

	type symbolPath struct {
		stamps []int64
		open   []float64
		high   []float64
		low    []float64
		close  []float64
		volume []float64
		weight float64 // quote-volume proxy over the window
	}

	var mu sync.Mutex
	var paths []symbolPath

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
				return
			}

			candles := cs.All()
			base := candles[0].Close()
			if base == 0 {
				return
			}
			factor := 100.0 / base

			n := len(candles)
			p := symbolPath{
				stamps: make([]int64, n),
				open:   make([]float64, n),
				high:   make([]float64, n),
				low:    make([]float64, n),
				close:  make([]float64, n),
				volume: make([]float64, n),
			}
			var quoteVol float64
			for i, c := range candles {
				p.stamps[i] = c.Timestamp().Unix()
				p.open[i] = c.Open() * factor
				p.high[i] = c.High() * factor
				p.low[i] = c.Low() * factor
				p.close[i] = c.Close() * factor
				p.volume[i] = c.Volume()
				quoteVol += c.Volume() * c.Close()
			}
			if quoteVol <= 0 {
				quoteVol = 1 // equal fallback when volume is missing
			}
			p.weight = quoteVol

			mu.Lock()
			paths = append(paths, p)
			mu.Unlock()
		}()
	}
	wg.Wait()

	empty := CompositeTape{
		Index:           mkt.CompositeIndex{Timeframe: timeframe, SymbolCount: len(symbols)},
		PreferredSource: "composite_median",
	}
	if len(paths) == 0 {
		return empty, nil
	}

	ref := paths[0]
	for _, p := range paths[1:] {
		if len(p.stamps) > len(ref.stamps) {
			ref = p
		}
	}

	medianPts := make([]mkt.IndexPoint, 0, len(ref.stamps))
	weightedPts := make([]mkt.IndexPoint, 0, len(ref.stamps))
	medianCandles := make([]domain.Candle, 0, len(ref.stamps))
	weightedCandles := make([]domain.Candle, 0, len(ref.stamps))

	synthSym := domain.NewSymbolUnsafe("COMPOSITE")

	for i, ts := range ref.stamps {
		var opens, highs, lows, closes, vols []float64
		var wSum, wOpen, wHigh, wLow, wClose float64
		for _, p := range paths {
			if i >= len(p.close) {
				continue
			}
			opens = append(opens, p.open[i])
			highs = append(highs, p.high[i])
			lows = append(lows, p.low[i])
			closes = append(closes, p.close[i])
			vols = append(vols, p.volume[i])
			w := p.weight
			wSum += w
			wOpen += p.open[i] * w
			wHigh += p.high[i] * w
			wLow += p.low[i] * w
			wClose += p.close[i] * w
		}
		if len(closes) == 0 {
			continue
		}

		mClose := median(append([]float64(nil), closes...))
		mOpen := median(append([]float64(nil), opens...))
		mHigh := median(append([]float64(nil), highs...))
		mLow := median(append([]float64(nil), lows...))
		mVol := sum(vols)
		if mHigh < mClose {
			mHigh = mClose
		}
		if mHigh < mOpen {
			mHigh = mOpen
		}
		if mLow > mClose {
			mLow = mClose
		}
		if mLow > mOpen {
			mLow = mOpen
		}

		medianPts = append(medianPts, mkt.IndexPoint{Timestamp: ts, Value: mClose})
		t := time.Unix(ts, 0).UTC()
		medianCandles = append(medianCandles, domain.NewCandleUnsafe(
			synthSym, tf, t, mOpen, mHigh, mLow, mClose, mVol,
		))

		if wSum > 0 {
			wC := wClose / wSum
			wO := wOpen / wSum
			wH := wHigh / wSum
			wL := wLow / wSum
			if wH < wC {
				wH = wC
			}
			if wH < wO {
				wH = wO
			}
			if wL > wC {
				wL = wC
			}
			if wL > wO {
				wL = wO
			}
			weightedPts = append(weightedPts, mkt.IndexPoint{Timestamp: ts, Value: wC})
			weightedCandles = append(weightedCandles, domain.NewCandleUnsafe(
				synthSym, tf, t, wO, wH, wL, wC, mVol,
			))
		}
	}

	medianSeries, _ := domain.NewCandleSeries(synthSym, tf, medianCandles)
	weightedSeries, _ := domain.NewCandleSeries(synthSym, tf, weightedCandles)

	source := "composite_median"
	if weightedSeries.Len() >= 2 {
		source = "composite_volume_weighted"
	}

	return CompositeTape{
		Index: mkt.CompositeIndex{
			Timeframe:            timeframe,
			Points:               medianPts,
			VolumeWeightedPoints: weightedPts,
			SymbolCount:          len(paths),
		},
		MedianSeries:    medianSeries,
		WeightedSeries:  weightedSeries,
		PreferredSource: source,
	}, nil
}

func median(vals []float64) float64 {
	sort.Float64s(vals)
	n := len(vals)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

func sum(vals []float64) float64 {
	var s float64
	for _, v := range vals {
		s += v
	}
	return s
}
