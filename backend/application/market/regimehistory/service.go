package regimehistory

import (
	"context"
	"time"

	"pano_chart/backend/application/replay"
	mkt "pano_chart/backend/domain/market"
)

// Service provides read access to regime history.
type Service struct {
	repo Repository
}

// NewService constructs the history service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetHistory returns the regime history for a timeframe, including the
// current regime age (duration of the most recent period).
// When ctx carries replay AsOf (PR-112a), returns a point-in-time view:
// closed predecessors plus the period covering asOf with age at asOf.
func (s *Service) GetHistory(ctx context.Context, timeframe string, limit int) (mkt.RegimeHistory, error) {
	fetchLimit := limit
	if _, ok := replay.AsOf(ctx); ok {
		fetchLimit = historyFetchLimit(limit)
	}
	periods, err := s.repo.GetHistory(ctx, timeframe, fetchLimit)
	if err != nil {
		return mkt.RegimeHistory{}, err
	}

	age := 0
	if asOf, ok := replay.AsOf(ctx); ok {
		periods, age = historyAtAsOf(periods, timeframe, asOf.Unix(), limit)
	} else if len(periods) > 0 {
		age = periods[len(periods)-1].DurationCandles
	}

	return mkt.RegimeHistory{
		Timeframe:  timeframe,
		Periods:    periods,
		CurrentAge: age,
	}, nil
}

// AgeAtAsOf returns regime age in candles at a replay cutoff.
func (s *Service) AgeAtAsOf(ctx context.Context, timeframe string, asOf time.Time) (int, error) {
	periods, err := s.repo.GetHistory(ctx, timeframe, historyFetchLimit(50))
	if err != nil {
		return 0, err
	}
	_, age := historyAtAsOf(periods, timeframe, asOf.Unix(), 50)
	return age, nil
}

// CurrentAge returns just the age of the current regime in candles.
// Convenience method used by the transition engine.
func (s *Service) CurrentAge(timeframe string) (int, error) {
	latest, err := s.repo.GetLatest(timeframe)
	if err != nil {
		return 0, err
	}
	if latest == nil {
		return 0, nil
	}
	return latest.DurationCandles, nil
}

// OpenPeriodStart returns the StartTimestamp of the open (latest) period.
// Used so transition merged-age prefix only applies to the same open period.
func (s *Service) OpenPeriodStart(timeframe string) (int64, error) {
	latest, err := s.repo.GetLatest(timeframe)
	if err != nil {
		return 0, err
	}
	if latest == nil {
		return 0, nil
	}
	return latest.StartTimestamp, nil
}
