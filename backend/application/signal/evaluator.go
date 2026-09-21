package signal

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
	domainsignal "pano_chart/backend/domain/signal"
)

const (
	// EvalInterval is how often the job wakes (ROADMAP PR-091).
	EvalInterval = 5 * time.Minute
	// MaxPerTick caps successful resolves per tick.
	MaxPerTick = 200
	// readyFetchCap over-fetches ready rows so retryable skips (incomplete
	// path) do not starve other ready signals within the same tick.
	readyFetchCap = MaxPerTick * 5
	// tapeLimitCap bounds CalculateTape; beyond this a delayed market-wide
	// window is treated as permanently unavailable.
	tapeLimitCap = 500
	// regimeHistoryLimit is how many periods to load per TF for transition
	// grading. Tracker retains indefinitely; 500 covers multi-year 1d and
	// weeks of 15m transitions — raise if RegimeAt misses on long horizons.
	regimeHistoryLimit = 500
	// invalidDrainCap bounds never-ready (bad TF) poison marks per tick.
	invalidDrainCap = 50
)

var (
	errNoCandles         = errors.New("no horizon candles")
	errIncompletePath    = errors.New("incomplete horizon path")
	errInsufficientATR   = errors.New("insufficient atr")
	errRegimeUnavailable = errors.New("regime history unavailable")
	errTapeUnavailable   = errors.New("tape window unavailable")
	errPermanentTapeMiss = errors.New("tape window permanently unavailable")
)

// TapeSource supplies composite tapes for market-wide signals.
type TapeSource interface {
	CalculateTape(ctx context.Context, timeframe string, limit int) (metrics.CompositeTape, error)
}

// RegimeHistorySource looks up past regimes for transition grading.
type RegimeHistorySource interface {
	GetHistory(timeframe string, limit int) (mkt.RegimeHistory, error)
}

// Evaluator grades unresolved signals whose horizon has elapsed.
type Evaluator struct {
	repo     ports.SignalRepository
	candles  ports.CandleRepositoryPort
	tape     TapeSource
	regimes  RegimeHistorySource
	now      func() time.Time
	interval time.Duration
}

// NewEvaluator constructs the job. repo and candles are required for symbol
// signals; tape/regimes are optional (market-wide / transition).
func NewEvaluator(repo ports.SignalRepository, candles ports.CandleRepositoryPort) *Evaluator {
	return &Evaluator{
		repo:     repo,
		candles:  candles,
		now:      time.Now,
		interval: EvalInterval,
	}
}

// SetTapeProvider enables market-wide (empty symbol) path evaluation.
func (e *Evaluator) SetTapeProvider(tp TapeSource) {
	if e != nil {
		e.tape = tp
	}
}

// SetRegimeHistory enables transition:X grading via regime history.
func (e *Evaluator) SetRegimeHistory(src RegimeHistorySource) {
	if e != nil {
		e.regimes = src
	}
}

// SetNow overrides the clock (tests).
func (e *Evaluator) SetNow(fn func() time.Time) {
	if e != nil && fn != nil {
		e.now = fn
	}
}

// SetInterval overrides the Run tick (tests).
func (e *Evaluator) SetInterval(d time.Duration) {
	if e != nil && d > 0 {
		e.interval = d
	}
}

// Run ticks until ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	if e == nil || e.repo == nil {
		return
	}
	t := time.NewTicker(e.interval)
	defer t.Stop()
	e.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.Tick(ctx)
		}
	}
}

// Tick resolves up to MaxPerTick ready signals and drains invalid-TF rows.
func (e *Evaluator) Tick(ctx context.Context) int {
	if e == nil || e.repo == nil {
		return 0
	}
	now := e.now().UTC()
	e.drainInvalid(ctx, now)

	sigs, err := e.repo.UnresolvedReady(ctx, now, readyFetchCap)
	if err != nil {
		log.Printf("[signal-eval] unresolved ready: %v", err)
		return 0
	}

	// One CalculateTape per timeframe for this tick (max limit needed).
	tapeCache := e.prefetchTapes(ctx, sigs, now)

	resolved := 0
	for _, sig := range sigs {
		if ctx.Err() != nil {
			break
		}
		if resolved >= MaxPerTick {
			break
		}
		outcome, err := e.gradeOne(ctx, sig, now, tapeCache)
		if err != nil {
			if errors.Is(err, errIncompletePath) ||
				errors.Is(err, errInsufficientATR) ||
				errors.Is(err, errRegimeUnavailable) ||
				errors.Is(err, errTapeUnavailable) ||
				errors.Is(err, errNoCandles) {
				// Retryable: leave unresolved, try next ready row.
				continue
			}
			if errors.Is(err, errPermanentTapeMiss) {
				_ = e.repo.MarkResolved(ctx, sig.ID, domainsignal.Outcome{
					SignalID:   sig.ID,
					ResolvedAt: now,
					Rule:       domainsignal.RulePathUnavailable,
				})
				resolved++
				continue
			}
			log.Printf("[signal-eval] id=%s label=%s: %v", sig.ID, sig.Label, err)
			continue
		}
		if err := e.repo.MarkResolved(ctx, sig.ID, outcome); err != nil {
			log.Printf("[signal-eval] mark %s: %v", sig.ID, err)
			continue
		}
		resolved++
	}
	if resolved > 0 {
		log.Printf("[signal-eval] resolved %d signal(s)", resolved)
	}
	return resolved
}

func (e *Evaluator) drainInvalid(ctx context.Context, now time.Time) {
	sigs, err := e.repo.Unresolved(ctx, now, invalidDrainCap)
	if err != nil {
		return
	}
	for _, sig := range sigs {
		if _, ok := domainsignal.HorizonEnd(sig); ok {
			continue
		}
		_ = e.repo.MarkResolved(ctx, sig.ID, domainsignal.Outcome{
			SignalID:   sig.ID,
			ResolvedAt: now,
			Rule:       domainsignal.RuleInvalid,
		})
	}
}

func (e *Evaluator) prefetchTapes(ctx context.Context, sigs []domainsignal.Signal, now time.Time) map[string]metrics.CompositeTape {
	if e.tape == nil {
		return nil
	}
	need := map[string]int{} // tf → max bars
	for _, sig := range sigs {
		if strings.TrimSpace(sig.Symbol) != "" {
			continue
		}
		tf, err := domain.NewTimeframe(sig.Timeframe)
		if err != nil || tf.Duration() == 0 {
			continue
		}
		bars := tapeBarsNeeded(sig.EmittedAt, now, tf.Duration(), domainsignal.HorizonBarsOf(sig))
		if bars > need[sig.Timeframe] {
			need[sig.Timeframe] = bars
		}
	}
	out := make(map[string]metrics.CompositeTape, len(need))
	for tf, bars := range need {
		if bars > tapeLimitCap {
			continue // permanent miss handled per-signal
		}
		tape, err := e.tape.CalculateTape(ctx, tf, bars)
		if err != nil {
			log.Printf("[signal-eval] tape prefetch tf=%s: %v", tf, err)
			continue
		}
		out[tf] = tape
	}
	return out
}

func tapeBarsNeeded(emitted, now time.Time, dur time.Duration, horizonBars int) int {
	// Cover [emitted, horizonEnd) plus lookback for SimpleATR(14).
	span := now.Sub(emitted.UTC())
	bars := int(span/dur) + horizonBars + 16
	if bars < horizonBars+16 {
		bars = horizonBars + 16
	}
	return bars
}

func (e *Evaluator) gradeOne(
	ctx context.Context,
	sig domainsignal.Signal,
	now time.Time,
	tapeCache map[string]metrics.CompositeTape,
) (domainsignal.Outcome, error) {
	end, ok := domainsignal.HorizonEnd(sig)
	if !ok {
		return domainsignal.Outcome{}, errIncompletePath
	}
	horizonBars := domainsignal.HorizonBarsOf(sig)

	closes, highs, lows, price, atr, err := e.loadPath(ctx, sig, end, now, tapeCache)
	if err != nil {
		return domainsignal.Outcome{}, err
	}
	if len(closes) < horizonBars {
		return domainsignal.Outcome{}, errIncompletePath
	}
	// Exact horizon window: drop any inclusive endTime extras after clip.
	if len(closes) > horizonBars {
		closes = closes[:horizonBars]
		highs = highs[:horizonBars]
		lows = lows[:horizonBars]
	}

	if price <= 0 {
		price = sig.Price
	}
	if atr <= 0 {
		atr = sig.ATR
	}
	if domainsignal.NeedsATR(sig.Label) && atr <= 0 {
		return domainsignal.Outcome{}, errInsufficientATR
	}
	sig.Price = price
	sig.ATR = atr

	regimeAt := ""
	if strings.HasPrefix(strings.ToLower(sig.Label), "transition:") {
		regimeAt, err = e.lookupRegime(sig.Timeframe, end)
		if err != nil {
			return domainsignal.Outcome{}, err
		}
	}

	dir := domainsignal.DirectionFromLabel(sig.Label)
	stats := domainsignal.ComputePathStats(price, atr, dir, closes, highs, lows)
	outcome := domainsignal.Grade(sig, stats, regimeAt)
	outcome.ResolvedAt = now
	return outcome, nil
}

func (e *Evaluator) loadPath(
	ctx context.Context,
	sig domainsignal.Signal,
	end, now time.Time,
	tapeCache map[string]metrics.CompositeTape,
) (closes, highs, lows []float64, price, atr float64, err error) {
	price, atr = sig.Price, sig.ATR
	from := sig.EmittedAt.UTC()
	to := end

	if strings.TrimSpace(sig.Symbol) == "" {
		return e.loadTapePath(sig.Timeframe, from, to, now, price, atr, tapeCache)
	}
	if e.candles == nil {
		return nil, nil, nil, 0, 0, errors.New("candle repository nil")
	}
	sym, symErr := domain.NewSymbol(sig.Symbol)
	if symErr != nil {
		return nil, nil, nil, 0, 0, symErr
	}
	tf, tfErr := domain.NewTimeframe(sig.Timeframe)
	if tfErr != nil {
		return nil, nil, nil, 0, 0, tfErr
	}
	series, serErr := e.candles.GetSeries(ctx, sym, tf, from, to)
	if serErr != nil {
		return nil, nil, nil, 0, 0, serErr
	}
	closes, highs, lows = extractOHLCClipped(series, from, to)
	return closes, highs, lows, price, atr, nil
}

func (e *Evaluator) loadTapePath(
	timeframe string,
	from, to, now time.Time,
	price, atr float64,
	tapeCache map[string]metrics.CompositeTape,
) (closes, highs, lows []float64, outPrice, outATR float64, err error) {
	outPrice, outATR = price, atr
	if e.tape == nil && tapeCache == nil {
		return nil, nil, nil, 0, 0, errTapeUnavailable
	}
	tf, tfErr := domain.NewTimeframe(timeframe)
	if tfErr != nil {
		return nil, nil, nil, 0, 0, tfErr
	}
	dur := tf.Duration()
	if dur <= 0 {
		return nil, nil, nil, 0, 0, errTapeUnavailable
	}
	bars := tapeBarsNeeded(from, now, dur, int(to.Sub(from)/dur))
	if bars > tapeLimitCap {
		return nil, nil, nil, 0, 0, errPermanentTapeMiss
	}

	tape, ok := tapeCache[timeframe]
	if !ok {
		return nil, nil, nil, 0, 0, errTapeUnavailable
	}
	all := tape.PreferredSeries().All()
	if len(all) == 0 {
		return nil, nil, nil, 0, 0, errNoCandles
	}
	oldest := all[0].Timestamp()
	if oldest.After(from) {
		// Latest-N tape does not reach EmittedAt — will not recover via CalculateTape.
		return nil, nil, nil, 0, 0, errPermanentTapeMiss
	}

	var lookback []domain.Candle
	for _, c := range all {
		ts := c.Timestamp()
		if ts.Before(from) {
			lookback = append(lookback, c)
			continue
		}
		if !ts.Before(to) {
			break
		}
		closes = append(closes, c.Close())
		highs = append(highs, c.High())
		lows = append(lows, c.Low())
	}
	if outPrice <= 0 {
		if len(lookback) > 0 {
			outPrice = lookback[len(lookback)-1].Close()
		} else if len(closes) > 0 {
			outPrice = closes[0]
		}
	}
	if outATR <= 0 && len(lookback) >= 2 {
		outATR = simpleATR(lookback, 14)
	}
	return closes, highs, lows, outPrice, outATR, nil
}

func extractOHLCClipped(series domain.CandleSeries, from, to time.Time) (closes, highs, lows []float64) {
	for _, c := range series.All() {
		ts := c.Timestamp()
		if ts.Before(from) || !ts.Before(to) {
			continue
		}
		closes = append(closes, c.Close())
		highs = append(highs, c.High())
		lows = append(lows, c.Low())
	}
	return closes, highs, lows
}

// simpleATR mirrors usecases.SimpleATR on a raw candle slice (emit-time helper).
func simpleATR(candles []domain.Candle, n int) float64 {
	length := len(candles)
	if length < 2 || n <= 0 {
		return 0
	}
	if n > length-1 {
		n = length - 1
	}
	sum := 0.0
	start := length - n
	for i := start; i < length; i++ {
		curr := candles[i]
		prev := candles[i-1]
		tr := math.Max(curr.High()-curr.Low(),
			math.Max(math.Abs(curr.High()-prev.Close()),
				math.Abs(curr.Low()-prev.Close())))
		sum += tr
	}
	return sum / float64(n)
}

func (e *Evaluator) lookupRegime(timeframe string, at time.Time) (string, error) {
	if e.regimes == nil {
		return "", errRegimeUnavailable
	}
	hist, err := e.regimes.GetHistory(timeframe, regimeHistoryLimit)
	if err != nil {
		return "", errRegimeUnavailable
	}
	if len(hist.Periods) == 0 {
		return "", errRegimeUnavailable
	}
	r, ok := mkt.RegimeAt(hist.Periods, at.Unix())
	if !ok {
		return "", errRegimeUnavailable
	}
	return string(r), nil
}
