package regimehistory

import (
	"sync"
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// Tracker is the core logic for recording regime transitions.
// It is called every time CalculateRegime completes, but only records
// one observation per candle boundary to avoid inflating duration counts.
type Tracker struct {
	repo Repository

	mu             sync.Mutex
	lastBoundaries map[string]int64 // timeframe → last recorded candle boundary (unix)
}

// NewTracker constructs the tracker.
func NewTracker(repo Repository) *Tracker {
	return &Tracker{
		repo:           repo,
		lastBoundaries: make(map[string]int64),
	}
}

// candleBoundary truncates a unix timestamp to the start of the candle
// period for the given timeframe string.
func candleBoundary(timeframe string, ts int64) int64 {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return ts // shouldn't happen; fall through
	}
	d := tf.Duration()
	if d == 0 {
		return ts
	}
	t := time.Unix(ts, 0).UTC()
	return t.Truncate(d).Unix()
}

// Update records a regime observation.  If the regime has changed, it closes
// the previous period and starts a new one.  Otherwise it increments the
// duration of the current period.
//
// Duplicate calls within the same candle boundary are no-ops (same regime)
// or trigger a transition (regime changed mid-candle).
func (t *Tracker) Update(timeframe string, newRegime mkt.Regime, timestamp int64) error {
	boundary := candleBoundary(timeframe, timestamp)

	t.mu.Lock()
	lastBoundary := t.lastBoundaries[timeframe]
	sameBoundary := boundary == lastBoundary && lastBoundary != 0
	t.mu.Unlock()

	current, err := t.repo.GetLatest(timeframe)
	if err != nil {
		return err
	}

	// First observation — start a fresh period.
	if current == nil {
		if err := t.repo.Append(timeframe, mkt.RegimePeriod{
			Regime:          newRegime,
			StartTimestamp:  timestamp,
			DurationCandles: 1,
		}); err != nil {
			return err
		}
		t.mu.Lock()
		t.lastBoundaries[timeframe] = boundary
		t.mu.Unlock()
		return nil
	}

	// Same regime — only increment if we crossed a new candle boundary.
	if current.Regime == newRegime {
		if sameBoundary {
			return nil // duplicate call within same candle — skip
		}
		if err := t.repo.UpdateDuration(timeframe, current.DurationCandles+1); err != nil {
			return err
		}
		t.mu.Lock()
		t.lastBoundaries[timeframe] = boundary
		t.mu.Unlock()
		return nil
	}

	// Regime changed — close old, open new (always honour transitions).
	if err := t.repo.CloseCurrent(timeframe, timestamp); err != nil {
		return err
	}
	if err := t.repo.Append(timeframe, mkt.RegimePeriod{
		Regime:          newRegime,
		StartTimestamp:  timestamp,
		DurationCandles: 1,
	}); err != nil {
		return err
	}
	t.mu.Lock()
	t.lastBoundaries[timeframe] = boundary
	t.mu.Unlock()
	return nil
}
