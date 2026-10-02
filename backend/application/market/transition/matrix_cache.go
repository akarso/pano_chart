package transition

import (
	"context"
	"sync"
	"time"

	mkt "pano_chart/backend/domain/market"
)

// PeriodHistory supplies regime periods for empirical matrix builds (PR-107).
type PeriodHistory interface {
	GetHistory(ctx context.Context, timeframe string, limit int) ([]mkt.RegimePeriod, error)
}

// MatrixView is the cache result for a timeframe.
type MatrixView struct {
	Matrix       Matrix
	MergedAge    int        // trailing merged core-regime age from last successful periods
	MergedRegime mkt.Regime // core regime of that trailing run (empty if none)
	Stale        bool       // true when serving a matrix after a failed refresh
}

// MatrixCache rebuilds an empirical Matrix per timeframe on a TTL.
// Rebuilds are keyed per timeframe so one TF's DB I/O does not block others.
// History fetches run without holding the slot lock and honour ctx cancellation.
// Failed refreshes mark the prior matrix stale (no blend) and negative-cache
// the failure for the TTL so outages do not retry every request.
type MatrixCache struct {
	history PeriodHistory
	limit   int
	ttl     time.Duration
	now     func() time.Time

	mu    sync.Mutex
	slots map[string]*matrixSlot
}

type matrixSlot struct {
	mu    sync.Mutex
	entry matrixCacheEntry
}

type matrixCacheEntry struct {
	matrix    Matrix
	periods   []mkt.RegimePeriod // last successful fetch (for merged age)
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

// Matrix returns a (possibly cached) empirical matrix for timeframe.
func (c *MatrixCache) Matrix(ctx context.Context, timeframe string) MatrixView {
	if c == nil || c.history == nil {
		return MatrixView{}
	}

	slot := c.slot(timeframe)
	nowFn := c.clock()

	slot.mu.Lock()
	now := nowFn()
	ent := slot.entry
	if !ent.builtAt.IsZero() && now.Sub(ent.builtAt) < c.ttl {
		age, reg := TrailingMerged(ent.periods)
		view := MatrixView{
			Matrix:       ent.matrix,
			MergedAge:    age,
			MergedRegime: reg,
			Stale:        ent.stale,
		}
		slot.mu.Unlock()
		return view
	}
	slot.mu.Unlock()

	// Fetch without holding the slot lock so other TFs / waiters are not blocked
	// for the duration of the DB read, and so ctx cancel can abort the wait path.
	periods, err := c.history.GetHistory(ctx, timeframe, c.limit)

	slot.mu.Lock()
	defer slot.mu.Unlock()
	now = nowFn()

	// Another goroutine may have refreshed while we fetched.
	if !slot.entry.builtAt.IsZero() && now.Sub(slot.entry.builtAt) < c.ttl && !slot.entry.stale {
		age, reg := TrailingMerged(slot.entry.periods)
		return MatrixView{
			Matrix:       slot.entry.matrix,
			MergedAge:    age,
			MergedRegime: reg,
			Stale:        false,
		}
	}

	if err != nil || ctx.Err() != nil {
		// Negative-cache the failure. Keep prior matrix but mark stale so
		// callers do not blend from unrevalidated history during an outage.
		if slot.entry.hasMatrix {
			slot.entry = matrixCacheEntry{
				matrix:    slot.entry.matrix,
				periods:   slot.entry.periods,
				builtAt:   now,
				stale:     true,
				hasMatrix: true,
			}
			age, reg := TrailingMerged(slot.entry.periods)
			return MatrixView{
				Matrix:       slot.entry.matrix,
				MergedAge:    age,
				MergedRegime: reg,
				Stale:        true,
			}
		}
		slot.entry = matrixCacheEntry{builtAt: now, stale: true}
		return MatrixView{Stale: true}
	}

	m := BuildMatrix(periods)
	slot.entry = matrixCacheEntry{
		matrix:    m,
		periods:   append([]mkt.RegimePeriod(nil), periods...),
		builtAt:   now,
		stale:     false,
		hasMatrix: true,
	}
	age, reg := TrailingMerged(periods)
	return MatrixView{
		Matrix:       m,
		MergedAge:    age,
		MergedRegime: reg,
		Stale:        false,
	}
}
