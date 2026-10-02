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

// MatrixView is the cache result for a timeframe.
type MatrixView struct {
	Matrix       Matrix
	MergedPrefix int        // trailing same-regime candles excluding the open period
	MergedRegime mkt.Regime // core regime of that trailing run (empty if none)
	Stale        bool       // true when serving a matrix after a failed refresh
}

// MatrixCache rebuilds an empirical Matrix per timeframe on a TTL.
// Rebuilds are keyed per timeframe so one TF's DB I/O does not block others.
// Concurrent misses coalesce via singleflight; the shared fetch uses
// WithoutCancel so one aborted client cannot abort siblings or poison the
// cache. Failed refreshes mark the prior matrix stale (no blend) and
// negative-cache the failure for the TTL so outages do not retry every request.
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
	return MatrixView{
		Matrix:       m,
		MergedPrefix: prefix,
		MergedRegime: reg,
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

	ch := c.group.DoChan(timeframe, func() (any, error) {
		if view, ok := c.hotView(slot, nowFn()); ok {
			return view, nil
		}

		// Detach from any single caller's cancel so one aborted HTTP client
		// cannot abort (or poison) a shared history read for siblings.
		periods, err := c.history.GetHistory(context.WithoutCancel(ctx), timeframe, c.limit)

		slot.mu.Lock()
		defer slot.mu.Unlock()
		now := nowFn()

		// Another flight may have refreshed while we fetched.
		if !slot.entry.builtAt.IsZero() && now.Sub(slot.entry.builtAt) < c.ttl && !slot.entry.stale {
			return viewFromPeriods(slot.entry.matrix, slot.entry.periods, false), nil
		}

		if err != nil {
			// Storage failure: negative-cache. Keep prior matrix but mark stale
			// so callers do not blend from unrevalidated history during an outage.
			if slot.entry.hasMatrix {
				slot.entry = matrixCacheEntry{
					matrix:    slot.entry.matrix,
					periods:   slot.entry.periods,
					builtAt:   now,
					stale:     true,
					hasMatrix: true,
				}
				return viewFromPeriods(slot.entry.matrix, slot.entry.periods, true), nil
			}
			slot.entry = matrixCacheEntry{builtAt: now, stale: true}
			return MatrixView{Stale: true}, nil
		}

		m := BuildMatrix(periods)
		slot.entry = matrixCacheEntry{
			matrix:    m,
			periods:   append([]mkt.RegimePeriod(nil), periods...),
			builtAt:   now,
			stale:     false,
			hasMatrix: true,
		}
		return viewFromPeriods(m, periods, false), nil
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
