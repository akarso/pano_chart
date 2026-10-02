package transition

import (
	"sync"
	"time"

	mkt "pano_chart/backend/domain/market"
)

// PeriodHistory supplies regime periods for empirical matrix builds (PR-107).
type PeriodHistory interface {
	GetHistory(timeframe string, limit int) ([]mkt.RegimePeriod, error)
}

// MatrixCache rebuilds an empirical Matrix per timeframe on a TTL.
// Rebuilds are keyed per timeframe so one TF's DB I/O does not block others.
// Failed fetches refresh builtAt (negative cache) so errors do not retry
// every request within the TTL.
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
	matrix  Matrix
	builtAt time.Time
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
func (c *MatrixCache) Matrix(timeframe string) Matrix {
	if c == nil || c.history == nil {
		return Matrix{}
	}

	slot := c.slot(timeframe)
	slot.mu.Lock()
	defer slot.mu.Unlock()

	now := c.clock()()
	ent := slot.entry
	if !ent.builtAt.IsZero() && now.Sub(ent.builtAt) < c.ttl {
		return ent.matrix
	}

	periods, err := c.history.GetHistory(timeframe, c.limit)
	if err != nil {
		// Negative-cache / keep prior: bump builtAt so we do not retry every call.
		slot.entry = matrixCacheEntry{
			matrix:  ent.matrix,
			builtAt: now,
		}
		return ent.matrix
	}
	m := BuildMatrix(periods)
	slot.entry = matrixCacheEntry{matrix: m, builtAt: now}
	return m
}
