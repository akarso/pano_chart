package setups_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/setups"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/setup"
)

// --- Fakes ---

type fakeCandleRepo struct {
	series domain.CandleSeries
	err    error
}

func (f *fakeCandleRepo) GetSeries(_ domain.Symbol, _ domain.Timeframe, _, _ time.Time) (domain.CandleSeries, error) {
	return f.series, f.err
}

func (f *fakeCandleRepo) GetLastNCandles(_ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

type fakeScorer struct {
	stats usecases.SymbolStats
	err   error
}

func (f *fakeScorer) Score(_ domain.CandleSeries) (usecases.SymbolStats, error) {
	if f.err != nil {
		return usecases.SymbolStats{}, f.err
	}
	return f.stats, nil
}

// --- Helpers ---

func makeSeries(n int) domain.CandleSeries {
	sym, _ := domain.NewSymbol("BTCUSDT")
	tf, _ := domain.NewTimeframe("4h")
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			100, 110, 90, 105, float64(1000+i*10),
		)
	}
	s, _ := domain.NewCandleSeries(sym, tf, candles)
	return s
}

// --- Tests ---

func TestSetupService_HappyPath(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 2.5,
		Scores: map[string]float64{
			"Compression":          0.8,
			"Trend Predictability": 0.6,
			"Sideways":             0.3,
		},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected symbol BTCUSDT, got %s", result.Symbol)
	}
	if result.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", result.Timeframe)
	}
	if len(result.Scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(result.Scores))
	}
	if result.BestSetup == "" {
		t.Error("expected a non-empty best setup")
	}
	if result.Score <= 0 {
		t.Errorf("expected positive best score, got %f", result.Score)
	}
}

func TestSetupService_InvalidSymbol(t *testing.T) {
	eng := setups.NewEngine()
	svc := setups.NewSetupService(&fakeCandleRepo{}, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for empty symbol")
	}
}

func TestSetupService_InvalidTimeframe(t *testing.T) {
	eng := setups.NewEngine()
	svc := setups.NewSetupService(&fakeCandleRepo{}, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestSetupService_CandleFetchError(t *testing.T) {
	repo := &fakeCandleRepo{err: errors.New("network failure")}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, &fakeScorer{}, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when candle fetch fails")
	}
}

func TestSetupService_ScorerError(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{err: errors.New("scoring failed")}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	_, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when scorer fails")
	}
}

func TestSetupService_EmptySeriesReturnsZeroScores(t *testing.T) {
	series := makeSeries(1) // Less than 2 candles
	repo := &fakeCandleRepo{series: series}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, &fakeScorer{}, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Scores) != 0 {
		t.Errorf("expected empty scores for short series, got %d", len(result.Scores))
	}
	if result.BestSetup != "" {
		t.Errorf("expected empty best setup, got %s", result.BestSetup)
	}
}

func TestSetupService_HighCompressionSelectsCompressionBreakout(t *testing.T) {
	series := makeSeries(50)
	repo := &fakeCandleRepo{series: series}
	scorer := &fakeScorer{stats: usecases.SymbolStats{
		TotalScore: 1.0,
		Scores: map[string]float64{
			"Compression":          0.95,
			"Trend Predictability": 0.1,
		},
	}}
	eng := setups.NewEngine()
	svc := setups.NewSetupService(repo, scorer, eng)

	result, err := svc.Evaluate(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BestSetup != setup.CompressionBreakout {
		t.Errorf("expected compression_breakout, got %s", result.BestSetup)
	}
}
