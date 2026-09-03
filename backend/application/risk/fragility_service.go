package risk

import (
	"context"
	"fmt"

	domainrisk "pano_chart/backend/domain/risk"
)

// MarketRiskData holds raw market data needed for fragility computation.
type MarketRiskData struct {
	Funding        float64
	OISeries       []float64
	LongRatio      float64
	Price          float64
	NearestCluster float64
}

// DataProvider abstracts the source of risk-relevant market data.
type DataProvider interface {
	Get(ctx context.Context, symbol, timeframe string) (MarketRiskData, error)
}

// Service orchestrates fragility computation for a single symbol.
type Service struct {
	engine   *Engine
	provider DataProvider
}

// NewService constructs the fragility service.
func NewService(engine *Engine, provider DataProvider) *Service {
	return &Service{engine: engine, provider: provider}
}

// Get fetches market data and computes the fragility assessment.
func (s *Service) Get(ctx context.Context, symbol, timeframe string) (domainrisk.Fragility, error) {
	if symbol == "" {
		return domainrisk.Fragility{}, fmt.Errorf("symbol is required")
	}
	if timeframe == "" {
		return domainrisk.Fragility{}, fmt.Errorf("timeframe is required")
	}

	data, err := s.provider.Get(ctx, symbol, timeframe)
	if err != nil {
		return domainrisk.Fragility{}, fmt.Errorf("data provider: %w", err)
	}

	components := s.engine.Calculate(
		data.Funding,
		data.OISeries,
		data.LongRatio,
		data.Price,
		data.NearestCluster,
	)

	score := FinalScore(components)
	side := DominantSide(data.Funding, data.LongRatio)

	return domainrisk.Fragility{
		Symbol:       symbol,
		Timeframe:    timeframe,
		Score:        score,
		RiskLevel:    RiskLevel(score),
		DominantSide: side,
		SqueezeRisk:  SqueezeRisk(side),
		Components:   components,
	}, nil
}
