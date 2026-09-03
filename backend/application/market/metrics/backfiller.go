package metrics

import (
	"context"
	"log"
	"sync"

	"pano_chart/backend/domain"
)

const (
	metricsWindow = 110 // candles per analysis window (matches sparkline precision)
	backfillFetch = 300 // max candles to fetch per symbol
)

// Backfiller seeds the regime history by walking through historical candle
// data and feeding computed regimes into the observer (typically the Tracker).
//
// Breadth values are computed with the same real domain/scoring calculators
// used in the live path, ensuring consistency between backfilled history and
// real-time regime detection.
type Backfiller struct {
	candles  CandleProvider
	observer RegimeObserver
}

// NewBackfiller constructs a Backfiller.
func NewBackfiller(cp CandleProvider, obs RegimeObserver) *Backfiller {
	return &Backfiller{candles: cp, observer: obs}
}

// Run performs a historical backfill for a single timeframe.
// steps controls how many candle periods to walk back (capped by available
// data).  The observer receives regime observations from oldest to newest.
func (b *Backfiller) Run(ctx context.Context, timeframe string, steps int) error {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return err
	}

	symbols, err := b.candles.Symbols(ctx)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return nil
	}

	// Fetch candles for all symbols with bounded parallelism.
	fetchN := metricsWindow + steps
	if fetchN > backfillFetch {
		fetchN = backfillFetch
	}

	var mu sync.Mutex
	var allCandles [][]domain.Candle
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup

	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cs, fetchErr := b.candles.GetLastNCandles(sym, tf, fetchN)
			if fetchErr != nil || cs.Len() < metricsWindow {
				return
			}

			mu.Lock()
			allCandles = append(allCandles, cs.All())
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(allCandles) == 0 {
		return nil
	}

	// Trim all series to the same length (aligned at the most-recent end).
	minLen := len(allCandles[0])
	for _, c := range allCandles[1:] {
		if len(c) < minLen {
			minLen = len(c)
		}
	}
	if minLen < metricsWindow {
		return nil
	}
	for i := range allCandles {
		n := len(allCandles[i])
		allCandles[i] = allCandles[i][n-minLen:]
	}

	numSteps := minLen - metricsWindow + 1
	if numSteps > steps {
		numSteps = steps
	}
	if numSteps <= 0 {
		return nil
	}

	// Walk from oldest window to newest.
	startIdx := minLen - metricsWindow - numSteps + 1

	for i := 0; i < numSteps; i++ {
		idx := startIdx + i

		var volValues []float64
		var returns []float64
		var windows [][]domain.Candle

		for _, candles := range allCandles {
			window := candles[idx : idx+metricsWindow]
			windows = append(windows, window)

			volValues = append(volValues, volatilityExpansion(window))

			first := window[0].Close()
			last := window[len(window)-1].Close()
			ret := 0.0
			if first != 0 {
				ret = (last - first) / first
			}
			returns = append(returns, ret)
		}

		if len(volValues) == 0 {
			continue
		}

		vol := median(volValues)

		// Compute dispersion (for completeness) and real breadth.
		sum := 0.0
		for _, r := range returns {
			sum += r
		}
		marketRet := sum / float64(len(returns))
		_ = dispersion(returns, marketRet)

		breadth := scoreBreadthFromCandles(windows, timeframe)
		regime, _, _ := detectRegime(breadth, vol)

		// Timestamp: last candle in the window from the first symbol.
		ts := allCandles[0][idx+metricsWindow-1].Timestamp().Unix()

		if err := b.observer.Update(timeframe, regime, ts); err != nil {
			log.Printf("[backfiller] step %d/%d: %v", i+1, numSteps, err)
		}
	}

	return nil
}
