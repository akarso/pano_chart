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
	// MaxPerTick caps MarkResolved writes across invalid drain + ready grading.
	MaxPerTick = 200
	// readyFetchCap over-fetches ready rows so retryable skips do not starve
	// other ready signals within the same tick.
	readyFetchCap = MaxPerTick * 5
	// tapeLimitCap bounds CalculateTape; beyond this a delayed market-wide
	// window is treated as permanently unavailable.
	tapeLimitCap = 500
	// regimeHistoryLimit is how many periods to load per TF for transition
	// grading. Tracker retains indefinitely; 500 covers multi-year 1d and
	// weeks of 15m transitions — raise if RegimeAt misses on long horizons.
	regimeHistoryLimit = 500
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

// Tick resolves up to MaxPerTick signals (invalid-TF drain + ready grading
// share one budget).
func (e *Evaluator) Tick(ctx context.Context) int {
	if e == nil || e.repo == nil {
		return 0
	}
	now := e.now().UTC()
	budget := MaxPerTick

	n := e.resolveInvalid(ctx, now, budget)
	budget -= n
	if budget <= 0 || ctx.Err() != nil {
		e.logResolved(n)
		return n
	}

	m := e.resolveReady(ctx, now, budget)
	total := n + m
	e.logResolved(total)
	return total
}

func (e *Evaluator) logResolved(n int) {
	if n > 0 {
		log.Printf("[signal-eval] resolved %d signal(s)", n)
	}
}

// resolveInvalid marks unknown-timeframe rows as Rule=invalid, up to budget.
func (e *Evaluator) resolveInvalid(ctx context.Context, now time.Time, budget int) int {
	if budget <= 0 {
		return 0
	}
	sigs, err := e.repo.UnresolvedInvalidTF(ctx, budget)
	if err != nil {
		log.Printf("[signal-eval] unresolved invalid tf: %v", err)
		return 0
	}
	resolved := 0
	for _, sig := range sigs {
		if ctx.Err() != nil || resolved >= budget {
			break
		}
		oc := domainsignal.Outcome{
			SignalID:   sig.ID,
			ResolvedAt: now,
			Rule:       domainsignal.RuleInvalid,
		}
		if err := e.repo.MarkResolved(ctx, sig.ID, oc); err != nil {
			log.Printf("[signal-eval] mark invalid %s: %v", sig.ID, err)
			continue
		}
		resolved++
	}
	return resolved
}

// resolveReady grades horizon-elapsed signals, up to budget MarkResolved writes.
func (e *Evaluator) resolveReady(ctx context.Context, now time.Time, budget int) int {
	if budget <= 0 {
		return 0
	}
	fetch := readyFetchCap
	if fetch < budget {
		fetch = budget
	}
	sigs, err := e.repo.UnresolvedReady(ctx, now, fetch)
	if err != nil {
		log.Printf("[signal-eval] unresolved ready: %v", err)
		return 0
	}
	tapeCache := e.prefetchTapes(ctx, sigs, now)

	resolved := 0
	for _, sig := range sigs {
		if ctx.Err() != nil || resolved >= budget {
			break
		}
		if e.persistGrade(ctx, sig, now, tapeCache) {
			resolved++
		}
	}
	return resolved
}

// persistGrade grades one signal and writes the outcome when definitive.
// Returns true when MarkResolved ran (counts toward the tick budget).
func (e *Evaluator) persistGrade(
	ctx context.Context,
	sig domainsignal.Signal,
	now time.Time,
	tapeCache map[string]metrics.CompositeTape,
) bool {
	outcome, err := e.gradeOne(ctx, sig, now, tapeCache)
	if err != nil {
		return e.handleGradeError(ctx, sig, now, err)
	}
	if err := e.repo.MarkResolved(ctx, sig.ID, outcome); err != nil {
		log.Printf("[signal-eval] mark %s: %v", sig.ID, err)
		return false
	}
	return true
}

func (e *Evaluator) handleGradeError(
	ctx context.Context,
	sig domainsignal.Signal,
	now time.Time,
	err error,
) bool {
	if isRetryableGradeError(err) {
		return false
	}
	if errors.Is(err, errPermanentTapeMiss) {
		oc := domainsignal.Outcome{
			SignalID:   sig.ID,
			ResolvedAt: now,
			Rule:       domainsignal.RulePathUnavailable,
		}
		if markErr := e.repo.MarkResolved(ctx, sig.ID, oc); markErr != nil {
			log.Printf("[signal-eval] mark %s: %v", sig.ID, markErr)
			return false
		}
		return true
	}
	log.Printf("[signal-eval] id=%s label=%s: %v", sig.ID, sig.Label, err)
	return false
}

func isRetryableGradeError(err error) bool {
	return errors.Is(err, errIncompletePath) ||
		errors.Is(err, errInsufficientATR) ||
		errors.Is(err, errRegimeUnavailable) ||
		errors.Is(err, errTapeUnavailable) ||
		errors.Is(err, errNoCandles)
}

func (e *Evaluator) prefetchTapes(ctx context.Context, sigs []domainsignal.Signal, now time.Time) map[string]metrics.CompositeTape {
	if e.tape == nil {
		return nil
	}
	need := map[string]int{}
	for _, sig := range sigs {
		if !domainsignal.NeedsPath(sig.Label) {
			continue
		}
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
			continue
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
	if !domainsignal.NeedsPath(sig.Label) {
		return e.gradeWithoutPath(sig, now)
	}
	return e.gradeWithPath(ctx, sig, now, tapeCache)
}

// gradeWithoutPath handles transition:* (regime history) and unsupported labels.
func (e *Evaluator) gradeWithoutPath(sig domainsignal.Signal, now time.Time) (domainsignal.Outcome, error) {
	label := strings.ToLower(strings.TrimSpace(sig.Label))
	regimeAt := ""
	if strings.HasPrefix(label, "transition:") {
		end, ok := domainsignal.HorizonEnd(sig)
		if !ok {
			// Unknown TF belongs in resolveInvalid; treat as retryable here.
			return domainsignal.Outcome{}, errIncompletePath
		}
		var err error
		regimeAt, err = e.lookupRegime(sig.Timeframe, end)
		if err != nil {
			return domainsignal.Outcome{}, err
		}
	}
	outcome := domainsignal.Grade(sig, domainsignal.PathStats{}, regimeAt)
	outcome.ResolvedAt = now
	return outcome, nil
}

func (e *Evaluator) gradeWithPath(
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

	dir := domainsignal.DirectionFromLabel(sig.Label)
	stats := domainsignal.ComputePathStats(price, atr, dir, closes, highs, lows)
	outcome := domainsignal.Grade(sig, stats, "")
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
	if all[0].Timestamp().After(from) {
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
