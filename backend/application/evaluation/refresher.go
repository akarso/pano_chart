package evaluation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// DefaultTimeframes are the scheduled evaluation windows. 1m/5m stay on-demand
// (PR-089a).
var DefaultTimeframes = []string{"15m", "1h", "4h", "1d"}

// errRefreshSkipped means another instance holds the lock — not a failure.
var errRefreshSkipped = errors.New("eval refresh skipped: lock not acquired")

// errStoreFresh means Redis already has a Put within the refresh interval —
// shared across replicas so losers do not re-score after the lock is released.
var errStoreFresh = errors.New("eval refresh skipped: store still fresh")

const (
	pollInterval = 15 * time.Second
	maxBackoff   = 15 * time.Minute
	baseBackoff  = 15 * time.Second

	// lockTTLFloor is a hard upper bound for cold full-universe scoring so the
	// lease cannot expire mid-Execute under plausible load. Release-on-done is
	// the primary schedule mechanism; TTL is a dead-holder safety net.
	lockTTLFloor     = 30 * time.Minute
	lockTTLPadding   = 30 * time.Second
	defaultLockKeyPx = "eval:refresh:"
)

// RefreshEnabledFromEnv interprets PC_EVAL_REFRESH. Unset/empty → enabled
// (ROADMAP default on). Explicit 0/false/no/off → disabled.
func RefreshEnabledFromEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// RankingsRunner is the rankings pipeline the refresher writes from.
type RankingsRunner interface {
	Execute(ctx context.Context, req usecases.GetRankingsRequest) (usecases.RankingsResult, error)
}

// RefreshLock is an optional distributed lease so only one API process scores
// a given timeframe at a time. Nil disables locking (single-process / tests).
type RefreshLock interface {
	// TryAcquire returns true if this holder now owns key until ttl elapses.
	TryAcquire(ctx context.Context, key string, ttl time.Duration, holder string) (bool, error)
	// Release deletes key only if its value still equals holder (compare-and-del).
	Release(ctx context.Context, key, holder string) error
}

// Refresher periodically computes rankings for each scheduled timeframe and
// Puts the resulting EvaluationSnapshots into an EvaluationStore.
//
// One timeframe is refreshed per Tick (staggered warm-up / load). Failed
// Execute/Put apply exponential backoff and do not advance lastPut.
// lastPut is stamped at Put completion (wall clock), not Tick start.
type Refresher struct {
	rankings   RankingsRunner
	store      ports.EvaluationStore
	lock       RefreshLock // optional
	holderID   string
	timeframes []string
	now        func() time.Time

	// lockTTLFor allows tests to shrink the lease below Execute duration.
	lockTTLFor func(tf string) time.Duration

	mu        sync.Mutex
	lastPut   map[string]time.Time
	inFlight  map[string]bool
	failCount map[string]int
	failUntil map[string]time.Time
	putCount  map[string]int
	skipCount map[string]int
}

// NewRefresher constructs a refresher. timeframes nil → DefaultTimeframes.
func NewRefresher(rankings RankingsRunner, store ports.EvaluationStore, timeframes []string) *Refresher {
	if len(timeframes) == 0 {
		timeframes = append([]string(nil), DefaultTimeframes...)
	}
	return &Refresher{
		rankings:   rankings,
		store:      store,
		holderID:   "local",
		timeframes: timeframes,
		now:        time.Now,
		lockTTLFor: DefaultLockTTL,
		lastPut:    make(map[string]time.Time),
		inFlight:   make(map[string]bool),
		failCount:  make(map[string]int),
		failUntil:  make(map[string]time.Time),
		putCount:   make(map[string]int),
		skipCount:  make(map[string]int),
	}
}

// SetLock attaches a distributed refresh lock (multi-instance).
func (r *Refresher) SetLock(lock RefreshLock, holderID string) {
	r.lock = lock
	if holderID != "" {
		r.holderID = holderID
	}
}

// SetNow overrides the clock (tests).
func (r *Refresher) SetNow(fn func() time.Time) {
	if fn != nil {
		r.now = fn
	}
}

// SetLockTTL overrides lease duration (tests — e.g. shorter than Execute).
func (r *Refresher) SetLockTTL(fn func(tf string) time.Duration) {
	if fn != nil {
		r.lockTTLFor = fn
	}
}

// RefreshInterval returns max(30s, tf/4) for a timeframe string.
func RefreshInterval(tf string) time.Duration {
	return domain.EvaluationRefreshInterval(domain.NewTimeframeUnsafe(tf))
}

// DefaultLockTTL is the lease duration used when SetLockTTL is not overridden:
// max(30m, refreshInterval+30s). The floor keeps cold full-universe scoring
// from outliving interval+padding; Release-on-done is the schedule mechanism.
func DefaultLockTTL(tf string) time.Duration {
	ttl := RefreshInterval(tf) + lockTTLPadding
	if ttl < lockTTLFloor {
		return lockTTLFloor
	}
	return ttl
}

// Tick refreshes at most one due timeframe. Concurrent Ticks for the same tf
// are serialized via inFlight; each call claims at most one TF.
func (r *Refresher) Tick(ctx context.Context) {
	now := r.now()
	for _, tf := range r.timeframes {
		if ctx.Err() != nil {
			return
		}
		if !r.tryClaim(tf, now) {
			continue
		}
		err := r.refreshOne(ctx, tf)
		// Stamp lastPut with completion wall time, not Tick-start now.
		r.release(tf, r.now(), err)
		if err != nil && !errors.Is(err, errRefreshSkipped) && !errors.Is(err, errStoreFresh) {
			log.Printf("[eval] refresh tf=%s: %v", tf, err)
		}
		return
	}
}

// Run loops until ctx is cancelled. Polls every 15s; no stampede warm-up —
// the first four polls each claim one TF.
func (r *Refresher) Run(ctx context.Context) {
	r.Tick(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Tick(ctx)
		}
	}
}

// PutCounts returns how many times Put succeeded per timeframe (tests).
func (r *Refresher) PutCounts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.putCount))
	for k, v := range r.putCount {
		out[k] = v
	}
	return out
}

func (r *Refresher) tryClaim(tf string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.inFlight[tf] {
		r.skipCount[tf]++
		return false
	}
	if until, ok := r.failUntil[tf]; ok && now.Before(until) {
		return false
	}
	if last, ok := r.lastPut[tf]; ok && now.Sub(last) < RefreshInterval(tf) {
		return false
	}
	r.inFlight[tf] = true
	return true
}

func (r *Refresher) release(tf string, completedAt time.Time, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inFlight, tf)
	if errors.Is(err, errRefreshSkipped) || errors.Is(err, errStoreFresh) {
		r.skipCount[tf]++
		return
	}
	if err != nil {
		r.failCount[tf]++
		shift := r.failCount[tf] - 1
		if shift > 10 {
			shift = 10
		}
		backoff := baseBackoff << uint(shift)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		r.failUntil[tf] = completedAt.Add(backoff)
		return
	}
	r.failCount[tf] = 0
	delete(r.failUntil, tf)
	r.lastPut[tf] = completedAt
	r.putCount[tf]++
}

func (r *Refresher) adoptStoreTime(tf string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastPut[tf] = at
}

func (r *Refresher) refreshOne(ctx context.Context, tf string) error {
	parsed, err := domain.NewTimeframe(tf)
	if err != nil {
		return err
	}
	release, err := r.acquireRefreshLock(ctx, tf)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}
	if err := r.skipIfStoreFresh(ctx, tf); err != nil {
		return err
	}
	return r.scoreAndPersist(ctx, parsed)
}

// acquireRefreshLock takes the per-TF lease. On success, release is non-nil and
// must be deferred; it uses an uncancellable context so shutdown cancel cannot
// strand the 30m floor.
func (r *Refresher) acquireRefreshLock(ctx context.Context, tf string) (release func(), err error) {
	if r.lock == nil {
		return nil, nil
	}
	lockKey := defaultLockKeyPx + tf
	ok, lockErr := r.lock.TryAcquire(ctx, lockKey, r.lockTTLFor(tf), r.holderID)
	if lockErr != nil {
		return nil, lockErr
	}
	if !ok {
		return nil, errRefreshSkipped
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if relErr := r.lock.Release(releaseCtx, lockKey, r.holderID); relErr != nil {
			log.Printf("[eval] release lock tf=%s: %v", tf, relErr)
		}
	}, nil
}

// skipIfStoreFresh returns errStoreFresh when Redis already has a Put within
// the refresh interval (shared across replicas after the lock is released).
func (r *Refresher) skipIfStoreFresh(ctx context.Context, tf string) error {
	_, at, getErr := r.store.Get(ctx, tf)
	if getErr == nil {
		if r.now().Sub(at) < RefreshInterval(tf) {
			r.adoptStoreTime(tf, at)
			return errStoreFresh
		}
		return nil
	}
	if errors.Is(getErr, ports.ErrEvaluationNotFound) {
		return nil
	}
	return getErr
}

// scoreAndPersist runs rankings and writes non-empty results to the store.
func (r *Refresher) scoreAndPersist(ctx context.Context, tf domain.Timeframe) error {
	out, err := r.rankings.Execute(ctx, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return err
	}
	results := out.Results
	if len(results) == 0 {
		// Do not Put [] — that would wipe the last good evaluations.
		return fmt.Errorf("rankings returned empty result for %s", tf)
	}
	completedAt := r.now()
	evals := appmarket.SnapshotsFromRankings(results, tf.String(), completedAt)
	if err := r.store.Put(ctx, tf.String(), evals, completedAt); err != nil {
		return err
	}
	log.Printf("[eval] put tf=%s n=%d", tf, len(evals))
	return nil
}
