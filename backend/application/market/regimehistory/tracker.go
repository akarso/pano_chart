package regimehistory

import mkt "pano_chart/backend/domain/market"

// Tracker is the core logic for recording regime transitions.
// It is called every time CalculateRegime completes.
type Tracker struct {
	repo Repository
}

// NewTracker constructs the tracker.
func NewTracker(repo Repository) *Tracker {
	return &Tracker{repo: repo}
}

// Update records a regime observation.  If the regime has changed, it closes
// the previous period and starts a new one.  Otherwise it increments the
// duration of the current period.
func (t *Tracker) Update(timeframe string, newRegime mkt.Regime, timestamp int64) error {
	current, err := t.repo.GetLatest(timeframe)
	if err != nil {
		return err
	}

	// First observation — start a fresh period.
	if current == nil {
		return t.repo.Append(timeframe, mkt.RegimePeriod{
			Regime:          newRegime,
			StartTimestamp:  timestamp,
			DurationCandles: 1,
		})
	}

	// Same regime — increment duration.
	if current.Regime == newRegime {
		return t.repo.UpdateDuration(timeframe, current.DurationCandles+1)
	}

	// Regime changed — close old, open new.
	if err := t.repo.CloseCurrent(timeframe, timestamp); err != nil {
		return err
	}
	return t.repo.Append(timeframe, mkt.RegimePeriod{
		Regime:          newRegime,
		StartTimestamp:  timestamp,
		DurationCandles: 1,
	})
}
