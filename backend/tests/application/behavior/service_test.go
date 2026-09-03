package behavior_test

import (
	"context"
	"errors"
	"testing"

	appbehavior "pano_chart/backend/application/behavior"
)

// --- Fake data provider ---

type fakeProvider struct {
	data appbehavior.BehaviorData
	err  error
}

func (f *fakeProvider) Get(_ context.Context, _, _ string) (appbehavior.BehaviorData, error) {
	return f.data, f.err
}

// --- Service tests ---

func TestService_HappyPath(t *testing.T) {
	provider := &fakeProvider{
		data: appbehavior.BehaviorData{
			FragilityScore:     0.5,
			FundingExtremeness: 0.6,
			OIExpansion:        0.4,
			Imbalance:          0.7,
			Regime:             "range",
			VolumeScore:        0.3,
			Volatility:         0.2,
		},
	}
	engine := appbehavior.NewEngine()
	svc := appbehavior.NewService(engine, provider)

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
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if result.Greed <= 0 {
		t.Errorf("expected positive greed, got %f", result.Greed)
	}
	for _, v := range []float64{result.Greed, result.Fear, result.Patience, result.Panic} {
		if v < 0 || v > 1 {
			t.Errorf("value out of [0,1]: %f", v)
		}
	}
}

func TestService_EmptySymbol(t *testing.T) {
	svc := appbehavior.NewService(appbehavior.NewEngine(), &fakeProvider{})
	_, err := svc.Get(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for empty symbol")
	}
}

func TestService_EmptyTimeframe(t *testing.T) {
	svc := appbehavior.NewService(appbehavior.NewEngine(), &fakeProvider{})
	_, err := svc.Get(context.Background(), "BTCUSDT", "")
	if err == nil {
		t.Fatal("expected error for empty timeframe")
	}
}

func TestService_ProviderError(t *testing.T) {
	provider := &fakeProvider{err: errors.New("provider down")}
	svc := appbehavior.NewService(appbehavior.NewEngine(), provider)
	_, err := svc.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error from provider")
	}
}
