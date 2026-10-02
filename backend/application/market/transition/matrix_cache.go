package transition

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	mkt "pano_chart/backend/domain/market"
)

// PeriodHistory supplies regime periods for empirical matrix builds (PR-107).
type PeriodHistory interface {
	GetHistory(ctx context.Context, timeframe string, limit int) ([]mkt.RegimePeriod, error)
}

// historyRefreshTimeout bounds the shared DB read so a stalled query cannot
// block a timeframe's singleflight indefinitely after callers cancel.
const historyRefreshTimeout = 5 * time.Second

// MatrixView is the cache result for a timeframe.
type MatrixView struct {
	Matrix       Matrix
	MergedPrefix int        // trailing same-regime candles excluding the open period
	MergedRegime mkt.Regime // core regime of that trailing run (empty if none)
	OpenStart    int64      // StartTimestamp of the cached open period (0 if unknown)
	Stale        bool       // true when serving a matrix after a failed refresh
}

// MatrixCache rebuilds an empirical Matrix per timeframe on a TTL.
// Rebuilds are keyed per timeframe so one TF's DB I/O does not block others.
// Concurrent misses coalesce via singleflight; the shared fetch uses a bounded
// timeout detached from caller cancel so one aborted client cannot abort
// siblings or poison the cache. Failed refreshes mark the prior matrix stale
// (no blend) and negative-cache the failure for the TTL.
type MatrixCache struct {
	history PeriodHistory
	limit   int
	ttl     time.Duration
	now     func() time.Time

	mu    sync.Mutex
	slots map[string]*matrixSlot
	group singleflight.Group
}

type matrixSlot struct {
	mu    sync.Mutex
	entry matrixCacheEntry
}

type matrixCacheEntry struct {
	matrix    Matrix
	periods   []mkt.RegimePeriod // last successful fetch (for merged-age prefix)
	builtAt   time.Time
	stale     bool
	hasMatrix bool
}

// NewMatrixCache constructs a cache. ttl defaults to 15m; limit to 500.
func NewMatrixCache(history PeriodHistory, ttl time.Duration, limit int) *MatrixCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if limit <= 0 {
		limit = 500
	}
	return &MatrixCache{
		history: history,
		limit:   limit,
		ttl:     ttl,
		now:     time.Now,
		slots:   make(map[string]*matrixSlot),
	}
}

// SetClock overrides the time source (tests).
func (c *MatrixCache) SetClock(now func() time.Time) {
	if c == nil || now == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *MatrixCache) slot(timeframe string) *matrixSlot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.slots[timeframe]
	if !ok {
		s = &matrixSlot{}
		c.slots[timeframe] = s
	}
	return s
}

func (c *MatrixCache) clock() func() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func viewFromPeriods(m Matrix, periods []mkt.RegimePeriod, stale bool) MatrixView {
	prefix, reg := TrailingMergedPrefix(periods)
	var openStart int64
	if n := len(periods); n > 0 {
		openStart = periods[n-1].StartTimestamp
	}
	return MatrixView{
		Matrix:       m,
		MergedPrefix: prefix,
		MergedRegime: reg,
		OpenStart:    openStart,
		Stale:        stale,
	}
}

func (c *MatrixCache) hotView(slot *matrixSlot, now time.Time) (MatrixView, bool) {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	ent := slot.entry
	if ent.builtAt.IsZero() || now.Sub(ent.builtAt) >= c.ttl {
		return MatrixView{}, false
	}
	return viewFromPeriods(ent.matrix, ent.periods, ent.stale), true
}

func (c *MatrixCache) priorView(slot *matrixSlot) MatrixView {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	ent := slot.entry
	if !ent.hasMatrix {
		return MatrixView{Stale: true}
	}
	// Caller aborted: serve prior matrix without marking the slot stale so
	// healthy siblings can still blend (or complete an in-flight refresh).
	return viewFromPeriods(ent.matrix, ent.periods, ent.stale)
}

// Matrix returns a (possibly cached) empirical matrix for timeframe.
func (c *MatrixCache) Matrix(ctx context.Context, timeframe string) MatrixView {
	if c == nil || c.history == nil {
		return MatrixView{}
	}
	slot := c.slot(timeframe)
	nowFn := c.clock()
	if view, ok := c.hotView(slot, nowFn()); ok {
		return view
	}
	return c.awaitRefresh(ctx, timeframe, slot, nowFn)
}

func (c *MatrixCache) awaitRefresh(
	ctx context.Context,
	timeframe string,
	slot *matrixSlot,
	nowFn func() time.Time,
) MatrixView {
	ch := c.group.DoChan(timeframe, func() (any, error) {
		return c.refresh(timeframe, slot, nowFn), nil
	})
	select {
	case <-ctx.Done():
		return c.priorView(slot)
	case res := <-ch:
		if res.Val == nil {
			return c.priorView(slot)
		}
		return res.Val.(MatrixView)
	}
}

func (c *MatrixCache) refresh(
	timeframe string,
	slot *matrixSlot,
	nowFn func() time.Time,
) MatrixView {
	if view, ok := c.hotView(slot, nowFn()); ok {
		return view
	}
	periods, err := c.loadHistory(timeframe)
	return c.commitRefresh(slot, nowFn, periods, err)
}

func (c *MatrixCache) loadHistory(timeframe string) ([]mkt.RegimePeriod, error) {
	ctx, cancel := context.WithTimeout(context.Background(), historyRefreshTimeout)
	defer cancel()
	return c.history.GetHistory(ctx, timeframe, c.limit)
}

func (c *MatrixCache) commitRefresh(
	slot *matrixSlot,
	nowFn func() time.Time,
	periods []mkt.RegimePeriod,
	err error,
) MatrixView {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	now := nowFn()

	// Another flight may have refreshed while we fetched.
	if !slot.entry.builtAt.IsZero() && now.Sub(slot.entry.builtAt) < c.ttl && !slot.entry.stale {
		return viewFromPeriods(slot.entry.matrix, slot.entry.periods, false)
	}

	if err != nil {
		return c.storeFailureLocked(slot, now)
	}

	m := BuildMatrix(periods)
	slot.entry = matrixCacheEntry{
		matrix:    m,
		periods:   append([]mkt.RegimePeriod(nil), periods...),
		builtAt:   now,
		stale:     false,
		hasMatrix: true,
	}
	return viewFromPeriods(m, periods, false)
}

func (c *MatrixCache) storeFailureLocked(slot *matrixSlot, now time.Time) MatrixView {
	// Storage failure: negative-cache. Keep prior matrix but mark stale so
	// callers do not blend from unrevalidated history during an outage.
	if slot.entry.hasMatrix {
		slot.entry = matrixCacheEntry{
			matrix:    slot.entry.matrix,
			periods:   slot.entry.periods,
			builtAt:   now,
			stale:     true,
			hasMatrix: true,
		}
		return viewFromPeriods(slot.entry.matrix, slot.entry.periods, true)
	}
	slot.entry = matrixCacheEntry{builtAt: now, stale: true}
	return MatrixView{Stale: true}
}
