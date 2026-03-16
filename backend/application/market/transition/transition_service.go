package transition

import (
	"context"
	"fmt"

	mkt "pano_chart/backend/domain/market"
)

// RegimeProvider abstracts the regime detection so the transition service
// can be tested without the full MetricsService dependency chain.
type RegimeProvider interface {
	CalculateRegime(ctx context.Context, timeframe string) (mkt.RegimeSummary, error)
}

// TransitionService orchestrates regime detection and transition-probability
// calculation.  It is the primary entry point for the HTTP handler.
type TransitionService struct {
	regimeProvider RegimeProvider
	engine         *TransitionEngine
}

// NewTransitionService wires the service.
func NewTransitionService(rp RegimeProvider, eng *TransitionEngine) *TransitionService {
	return &TransitionService{
		regimeProvider: rp,
		engine:         eng,
	}
}

// Calculate fetches the current regime summary and returns transition
// probabilities for the requested timeframe.
func (s *TransitionService) Calculate(ctx context.Context, timeframe string) (mkt.MarketTransition, error) {
	summary, err := s.regimeProvider.CalculateRegime(ctx, timeframe)
	if err != nil {
		return mkt.MarketTransition{}, fmt.Errorf("transition: regime error: %w", err)
	}

	// Derive volatility slope from the single-point VolatilityExpansion metric.
	// A value of 1.0 is neutral; >1 means expansion, <1 compression.
	volSlope := summary.Metrics.VolatilityExpansion - 1.0

	// TODO(PR-047): derive regimeAge from a historical window.
	const regimeAge = 12

	probs := s.engine.Calculate(
		summary.Regime,
		summary.Metrics.CompressionBreadth,
		volSlope,
		regimeAge,
	)

	return mkt.MarketTransition{
		Timeframe:     summary.Timeframe,
		CurrentRegime: summary.Regime,
		Probabilities: probs,
		Horizon:       fmt.Sprintf("%d candles", regimeAge),
	}, nil
}
