package signal

import (
	"context"
	"sync"
	"time"

	mkt "pano_chart/backend/domain/market"
	domainsignal "pano_chart/backend/domain/signal"
)

// RegimeHistoryLookup returns the latest stored regime for a timeframe.
// ok is false when no history exists. Used to lazy-seed on first Update.
type RegimeHistoryLookup func(timeframe string) (regime mkt.Regime, ok bool)

// RegimeWriter emits KindRegime when the tape regime changes for a timeframe.
// Satisfies market.RegimeObserver. Nil-safe when Emitter is nil / disabled.
type RegimeWriter struct {
	emitter *Emitter
	lookup  RegimeHistoryLookup
	mu      sync.Mutex
	last    map[string]mkt.Regime
	tfMu    map[string]*sync.Mutex // per-TF: compare → emit → commit
}

// NewRegimeWriter constructs the observer. emitter may be nil (no-op).
func NewRegimeWriter(emitter *Emitter) *RegimeWriter {
	return &RegimeWriter{
		emitter: emitter,
		last:    make(map[string]mkt.Regime),
		tfMu:    make(map[string]*sync.Mutex),
	}
}

// SetHistoryLookup enables lazy seed from regime history on first Update for
// a timeframe that was not Seed()'d at startup.
func (w *RegimeWriter) SetHistoryLookup(fn RegimeHistoryLookup) {
	if w == nil {
		return
	}
	w.lookup = fn
}

// Seed records the current regime without emitting (call on startup from
// regime history so process restarts do not fake a change).
func (w *RegimeWriter) Seed(timeframe string, regime mkt.Regime) {
	if w == nil || timeframe == "" || regime == "" {
		return
	}
	w.mu.Lock()
	w.last[timeframe] = regime
	w.mu.Unlock()
}

func (w *RegimeWriter) mutexFor(timeframe string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	m, ok := w.tfMu[timeframe]
	if !ok {
		m = &sync.Mutex{}
		w.tfMu[timeframe] = m
	}
	return m
}

// Update implements market.RegimeObserver.
// Per timeframe, compare → Emit → commit of last is one critical section so
// concurrent Updates cannot double-emit or commit out of order. last[tf]
// advances only after a successful Emit so Append failures retry.
func (w *RegimeWriter) Update(timeframe string, regime mkt.Regime, bias string, timestamp int64) error {
	if w == nil || !w.emitter.Enabled() {
		return nil
	}

	w.mu.Lock()
	_, seen := w.last[timeframe]
	lookup := w.lookup
	w.mu.Unlock()

	// History I/O outside the change-detection lock.
	if !seen && lookup != nil {
		if hist, ok := lookup(timeframe); ok {
			w.mu.Lock()
			if _, still := w.last[timeframe]; !still {
				w.last[timeframe] = hist
			}
			w.mu.Unlock()
		}
	}

	tfMu := w.mutexFor(timeframe)
	tfMu.Lock()
	defer tfMu.Unlock()

	w.mu.Lock()
	prev, seen := w.last[timeframe]
	if seen && prev == regime {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	at := time.Unix(timestamp, 0).UTC()
	if timestamp == 0 {
		at = time.Time{}
	}
	ctx := map[string]float64{}
	if bias == "up" {
		ctx["bias"] = 1
	} else if bias == "down" {
		ctx["bias"] = -1
	} else {
		ctx["bias"] = 0
	}
	ok := w.emitter.Emit(context.Background(), domainsignal.Signal{
		Kind:      domainsignal.KindRegime,
		Timeframe: timeframe,
		Label:     "regime:" + string(regime),
		Score:     1,
		EmittedAt: at,
		Context:   ctx,
	})
	if !ok {
		return nil
	}

	w.mu.Lock()
	w.last[timeframe] = regime
	w.mu.Unlock()
	return nil
}
