package behavior

import (
	"context"
	"fmt"

	domainbehavior "pano_chart/backend/domain/behavior"
)

// BehaviorData holds the raw signals needed to compute behavioral dimensions.
type BehaviorData struct {
	FragilityScore     float64
	FundingExtremeness float64
	OIExpansion        float64
	Imbalance          float64
	Regime             string
	VolumeScore        float64
	Volatility         float64
}

// DataProvider abstracts the source of behavior-relevant market data.
type DataProvider interface {
	Get(ctx context.Context, symbol, timeframe string) (BehaviorData, error)
}

// Service orchestrates behavior computation for a single symbol.
type Service struct {
	engine   *Engine
	provider DataProvider
}

// NewService constructs the behavior service.
func NewService(engine *Engine, provider DataProvider) *Service {
	return &Service{engine: engine, provider: provider}
}

// Get fetches market data and computes the retail behavior assessment.
func (s *Service) Get(ctx context.Context, symbol, timeframe string) (domainbehavior.RetailBehavior, error) {
	if symbol == "" {
		return domainbehavior.RetailBehavior{}, fmt.Errorf("symbol is required")
	}
	if timeframe == "" {
		return domainbehavior.RetailBehavior{}, fmt.Errorf("timeframe is required")
	}

	data, err := s.provider.Get(ctx, symbol, timeframe)
	if err != nil {
		return domainbehavior.RetailBehavior{}, fmt.Errorf("data provider: %w", err)
	}

	bctx := BehaviorContext(data)

	result := s.engine.Evaluate(bctx)
	result.Symbol = symbol
	result.Timeframe = timeframe

	return result, nil
}
