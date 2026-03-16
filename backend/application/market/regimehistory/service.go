package regimehistory

import mkt "pano_chart/backend/domain/market"

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
func (s *Service) GetHistory(timeframe string, limit int) (mkt.RegimeHistory, error) {
	periods, err := s.repo.GetHistory(timeframe, limit)
	if err != nil {
		return mkt.RegimeHistory{}, err
	}

	age := 0
	if len(periods) > 0 {
		age = periods[len(periods)-1].DurationCandles
	}

	return mkt.RegimeHistory{
		Timeframe:  timeframe,
		Periods:    periods,
		CurrentAge: age,
	}, nil
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
