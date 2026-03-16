package market

import (
	mkt "pano_chart/backend/domain/market"
)

// MarketStateService computes the aggregate market state summary
// by classifying each symbol's evaluation snapshot and computing
// breadth ratios.
type MarketStateService struct {
	provider EvaluationProvider
}

// NewMarketStateService constructs the service.
func NewMarketStateService(p EvaluationProvider) *MarketStateService {
	return &MarketStateService{provider: p}
}

// Calculate produces a market state summary for the given timeframe.
func (s *MarketStateService) Calculate(timeframe string) (mkt.Summary, error) {
	evaluations, err := s.provider.GetLatestEvaluations(timeframe)
	if err != nil {
		return mkt.Summary{}, err
	}

	if len(evaluations) == 0 {
		return mkt.Summary{
			Timeframe:   timeframe,
			State:       mkt.StateSideways,
			Confidence:  0,
			Breadth:     mkt.Breadth{},
			SymbolCount: 0,
		}, nil
	}

	counts := map[mkt.State]int{
		mkt.StateSideways:    0,
		mkt.StateCompression: 0,
		mkt.StateBreakout:    0,
		mkt.StateTrend:       0,
	}

	for _, e := range evaluations {
		state := classify(e)
		counts[state]++
	}

	total := float64(len(evaluations))

	breadth := mkt.Breadth{
		Sideways:    float64(counts[mkt.StateSideways]) / total,
		Compression: float64(counts[mkt.StateCompression]) / total,
		Breakout:    float64(counts[mkt.StateBreakout]) / total,
		Trend:       float64(counts[mkt.StateTrend]) / total,
	}

	dominant := mkt.StateSideways
	max := 0

	for st, c := range counts {
		if c > max {
			max = c
			dominant = st
		}
	}

	confidence := float64(max) / total

	return mkt.Summary{
		Timeframe:   timeframe,
		State:       dominant,
		Confidence:  confidence,
		Breadth:     breadth,
		SymbolCount: len(evaluations),
	}, nil
}
