package signal

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
	domainsignal "pano_chart/backend/domain/signal"
)

const (
	appendTimeout   = 2 * time.Second
	dedupeMapMax    = 10_000
	dedupeRetainFor = 48 * time.Hour
)

// Emitter appends signals best-effort with in-memory dedup keyed by
// (kind, symbol, timeframe, label) within the same candle boundary.
// A nil repository makes every method a no-op.
type Emitter struct {
	repo     ports.SignalRepository
	mu       sync.Mutex
	last     map[string]time.Time // key → EmittedAt of last successful Append
	inflight map[string]struct{}  // provisional claim while Append runs
	now      func() time.Time
}

// NewEmitter constructs an emitter. repo may be nil (no-op writers).
func NewEmitter(repo ports.SignalRepository) *Emitter {
	return &Emitter{
		repo:     repo,
		last:     make(map[string]time.Time),
		inflight: make(map[string]struct{}),
		now:      time.Now,
	}
}

// Enabled reports whether persistence is configured.
func (e *Emitter) Enabled() bool {
	return e != nil && e.repo != nil
}

// SetNow overrides the clock (tests).
func (e *Emitter) SetNow(fn func() time.Time) {
	if fn != nil {
		e.now = fn
	}
}

// Emit records s when the repository is set and the dedup window allows it.
// Persistence uses a detached short-timeout context so client cancel cannot
// abort the write. A provisional in-flight claim is taken under the lock,
// then Append runs unlocked so other keys are not blocked; last is updated
// only after a successful Append (claim rolled back on failure).
//
// Returns true when the signal is accounted for (persisted or already present
// this candle). Returns false on skip, Append failure, or a lost race to a
// peer already in flight for the same key — callers must only advance
// change-detection state on true.
func (e *Emitter) Emit(ctx context.Context, s domainsignal.Signal) bool {
	if !e.Enabled() {
		return false
	}
	if s.HorizonBars <= 0 {
		s.HorizonBars = domainsignal.DefaultHorizonBars
	}
	if s.EmittedAt.IsZero() {
		s.EmittedAt = e.now().UTC()
	}
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	// Symbol-scoped signals need a usable price/ATR for PR-091 MFE/MAE.
	if s.Symbol != "" && (s.Price <= 0 || s.ATR <= 0) {
		return false
	}

	key := dedupeKey(s.Kind, s.Symbol, s.Timeframe, s.Label)
	open := CandleOpen(s.Timeframe, s.EmittedAt)

	e.mu.Lock()
	if prev, ok := e.last[key]; ok && CandleOpen(s.Timeframe, prev).Equal(open) {
		e.mu.Unlock()
		return true
	}
	if _, busy := e.inflight[key]; busy {
		e.mu.Unlock()
		return false
	}
	e.inflight[key] = struct{}{}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.inflight, key)
		e.mu.Unlock()
	}()

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appendTimeout)
	err := e.repo.Append(persistCtx, s)
	cancel()
	if err != nil {
		log.Printf("[signal] append kind=%s symbol=%s tf=%s label=%s: %v",
			s.Kind, s.Symbol, s.Timeframe, s.Label, err)
		return false
	}

	e.mu.Lock()
	e.last[key] = s.EmittedAt
	e.evictLocked(e.now())
	e.mu.Unlock()
	return true
}

func dedupeKey(kind domainsignal.Kind, symbol, tf, label string) string {
	return string(kind) + "|" + symbol + "|" + tf + "|" + label
}

// CandleOpen truncates t to the start of the candle for tf (UTC).
// Unknown timeframes fall back to the raw second (no silent 1h window).
func CandleOpen(tfStr string, t time.Time) time.Time {
	t = t.UTC()
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		return t.Truncate(time.Second)
	}
	d := tf.Duration()
	if d <= 0 {
		return t.Truncate(time.Second)
	}
	return t.Truncate(d)
}

func (e *Emitter) evictLocked(now time.Time) {
	cutoff := now.Add(-dedupeRetainFor)
	for k, at := range e.last {
		if at.Before(cutoff) {
			delete(e.last, k)
		}
	}
	if len(e.last) <= dedupeMapMax {
		return
	}
	for len(e.last) > dedupeMapMax/2 {
		var oldestK string
		var oldestT time.Time
		first := true
		for k, at := range e.last {
			if first || at.Before(oldestT) {
				oldestK, oldestT, first = k, at, false
			}
		}
		if oldestK == "" {
			break
		}
		delete(e.last, oldestK)
	}
}
