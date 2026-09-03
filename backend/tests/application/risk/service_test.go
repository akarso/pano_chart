package risk_test

import (
	"context"
	"errors"
	"testing"

	"pano_chart/backend/application/risk"
	domainrisk "pano_chart/backend/domain/risk"
)

// --- Fake data provider ---

type fakeDataProvider struct {
	data risk.MarketRiskData
	err  error
}

func (f *fakeDataProvider) Get(_ context.Context, _, _ string) (risk.MarketRiskData, error) {
	if f.err != nil {
		return risk.MarketRiskData{}, f.err
	}
	return f.data, nil
}

func newService(data risk.MarketRiskData, err error) *risk.Service {
	eng := risk.NewEngine()
	return risk.NewService(eng, &fakeDataProvider{data: data, err: err})
}

// --- Tests ---

func TestService_HappyPath(t *testing.T) {
	data := risk.MarketRiskData{
		Funding:        0.005,
		OISeries:       []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 200},
		LongRatio:      0.7,
		Price:          100,
		NearestCluster: 105,
	}
	svc := newService(data, nil)

	result, err := svc.Get(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", result.Symbol)
	}
	if result.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", result.Timeframe)
	}
	if result.Score < 0 || result.Score > 1 {
		t.Errorf("score out of range: %f", result.Score)
	}
	validLevel := result.RiskLevel == "low" || result.RiskLevel == "medium" || result.RiskLevel == "high"
	if !validLevel {
		t.Errorf("unexpected risk level: %s", result.RiskLevel)
	}
	// With funding>0 and longRatio>0.6, dominant side should be long.
	if result.DominantSide != "long" {
		t.Errorf("expected dominantSide long, got %s", result.DominantSide)
	}
	if result.SqueezeRisk != "long_squeeze" {
		t.Errorf("expected squeezeRisk long_squeeze, got %s", result.SqueezeRisk)
	}
}

func TestService_EmptySymbol(t *testing.T) {
	svc := newService(risk.MarketRiskData{}, nil)

	_, err := svc.Get(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for empty symbol")
	}
}

func TestService_EmptyTimeframe(t *testing.T) {
	svc := newService(risk.MarketRiskData{}, nil)

	_, err := svc.Get(context.Background(), "BTCUSDT", "")
	if err == nil {
		t.Fatal("expected error for empty timeframe")
	}
}

func TestService_ProviderError(t *testing.T) {
	svc := newService(risk.MarketRiskData{}, errors.New("boom"))

	_, err := svc.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestService_ResultHasCorrectComponents(t *testing.T) {
	data := risk.MarketRiskData{
		Funding:        0.01,
		OISeries:       make([]float64, 10),
		LongRatio:      1.0,
		Price:          100,
		NearestCluster: 100,
	}
	svc := newService(data, nil)

	result, err := svc.Get(context.Background(), "ETHUSDT", "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	approx(t, "funding", result.Components.FundingExtremeness, 1.0)
	approx(t, "oi", result.Components.OIExpansion, 0)
	approx(t, "imbalance", result.Components.LongShortImbalance, 1.0)
	approx(t, "liquidation", result.Components.LiquidationProximity, 1.0)
}

func TestService_ScoreMatchesFinalScore(t *testing.T) {
	data := risk.MarketRiskData{
		Funding:        0.005,
		OISeries:       []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 200},
		LongRatio:      0.7,
		Price:          100,
		NearestCluster: 105,
	}
	svc := newService(data, nil)

	result, err := svc.Get(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := risk.FinalScore(domainrisk.FragilityComponents{
		FundingExtremeness:   result.Components.FundingExtremeness,
		OIExpansion:          result.Components.OIExpansion,
		LongShortImbalance:   result.Components.LongShortImbalance,
		LiquidationProximity: result.Components.LiquidationProximity,
	})
	approx(t, "score=FinalScore", result.Score, expected)
}

func TestService_RiskLevelMatchesScore(t *testing.T) {
	data := risk.MarketRiskData{
		Funding:        0.01,
		OISeries:       []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 300},
		LongRatio:      1.0,
		Price:          100,
		NearestCluster: 100,
	}
	svc := newService(data, nil)

	result, err := svc.Get(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := risk.RiskLevel(result.Score)
	if result.RiskLevel != expected {
		t.Errorf("expected risk level %s, got %s", expected, result.RiskLevel)
	}
}
