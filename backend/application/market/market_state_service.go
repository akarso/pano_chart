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
//
// Breadth is computed using proportional weighting: every symbol distributes
// its scores continuously across all four regimes (sideways, compression,
// breakout, trend).  This eliminates the zero-breadth problem that occurred
// with binary classification thresholds.
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

	// Accumulate proportional breadth across all tokens.
	var breadth mkt.Breadth
	for _, e := range evaluations {
		w := scoreWeights(e)
		breadth.Sideways += w.Sideways
		breadth.Compression += w.Compression
		breadth.Breakout += w.Breakout
		breadth.Trend += w.Trend
	}

	total := float64(len(evaluations))
	breadth.Sideways /= total
	breadth.Compression /= total
	breadth.Breakout /= total
	breadth.Trend /= total

	// Dominant state = highest weighted breadth.
	// Check order: sideways → trend → compression → breakout so that
	// higher-priority regimes win on ties (>= comparison).
	dominant := mkt.StateSideways
	maxWeight := breadth.Sideways

	if breadth.Trend >= maxWeight {
		dominant = mkt.StateTrend
		maxWeight = breadth.Trend
	}
	if breadth.Compression >= maxWeight {
		dominant = mkt.StateCompression
		maxWeight = breadth.Compression
	}
	if breadth.Breakout >= maxWeight {
		dominant = mkt.StateBreakout
		maxWeight = breadth.Breakout
	}

	return mkt.Summary{
		Timeframe:   timeframe,
		State:       dominant,
		Confidence:  maxWeight,
		Breadth:     breadth,
		SymbolCount: len(evaluations),
	}, nil
}
