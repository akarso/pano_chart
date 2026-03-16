package regimehistory

import mkt "pano_chart/backend/domain/market"

// Repository persists regime history periods.
type Repository interface {
	// GetLatest returns the most recent (possibly open) period for a timeframe.
	// Returns nil, nil when no history exists yet.
	GetLatest(timeframe string) (*mkt.RegimePeriod, error)

	// Append inserts a new open period.
	Append(timeframe string, period mkt.RegimePeriod) error

	// CloseCurrent sets the end timestamp on the currently open period.
	CloseCurrent(timeframe string, endTimestamp int64) error

	// UpdateDuration increments the duration of the currently open period.
	UpdateDuration(timeframe string, newDuration int) error

	// GetHistory returns the most recent `limit` periods, ordered oldest-first.
	GetHistory(timeframe string, limit int) ([]mkt.RegimePeriod, error)
}
