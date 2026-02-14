package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	symboluniverse "pano_chart/backend/application/symbol_universe"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type fakeSymbolUniverse struct {
	syms []domain.Symbol
}

func (f *fakeSymbolUniverse) Symbols(ctx context.Context, exchangeInfoURL, tickerURL string) ([]domain.Symbol, error) {
	return f.syms, nil
}

var _ symboluniverse.SymbolUniverseProvider = (*fakeSymbolUniverse)(nil)

type fakeCandleRepo struct {
	series map[domain.Symbol]domain.CandleSeries
	lastN  int
}

func (f *fakeCandleRepo) GetSeries(symbol domain.Symbol, timeframe domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	if s, ok := f.series[symbol]; ok {
		return s, nil
	}
	return domain.NewCandleSeries(symbol, timeframe, []domain.Candle{})
}

func (f *fakeCandleRepo) GetLastNCandles(symbol domain.Symbol, timeframe domain.Timeframe, n int) (domain.CandleSeries, error) {
	f.lastN = n
	if s, ok := f.series[symbol]; ok {
		return s, nil
	}
	return domain.NewCandleSeries(symbol, timeframe, []domain.Candle{})
}

func mustSeries(symbol domain.Symbol, tf domain.Timeframe, count int) domain.CandleSeries {
	candles := make([]domain.Candle, 0, count)
	base := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		c := domain.NewCandleUnsafe(symbol, tf, ts, 100, 110, 90, 105, 1000)
		candles = append(candles, c)
	}
	series, _ := domain.NewCandleSeries(symbol, tf, candles)
	return series
}

type fakeScorer struct {
	stats usecases.SymbolStats
	err   error
}

func (f *fakeScorer) Score(series domain.CandleSeries) (usecases.SymbolStats, error) {
	if f.err != nil {
		return usecases.SymbolStats{}, f.err
	}
	return f.stats, nil
}

func TestGetSymbolDetailReturnsCandlesAndScores(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	series := mustSeries(sym, tf, 3)

	repo := &fakeCandleRepo{series: map[domain.Symbol]domain.CandleSeries{sym: series}}
	universe := &fakeSymbolUniverse{syms: []domain.Symbol{sym}}
	scorer := &fakeScorer{stats: usecases.SymbolStats{TotalScore: 0.8, Scores: map[string]float64{"Gain/Loss": 0.8}}}

	uc := usecases.NewGetSymbolDetail(repo, scorer, universe, "x", "y", 200, 1000)
	result, err := uc.Execute(context.Background(), usecases.GetSymbolDetailRequest{
		Symbol:    sym,
		Timeframe: tf,
		Limit:     200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Candles) != 3 {
		t.Fatalf("expected 3 candles, got %d", len(result.Candles))
	}
	if result.Stats == nil || result.Stats.TotalScore != 0.8 {
		t.Fatalf("expected stats, got %+v", result.Stats)
	}
}

func TestGetSymbolDetailSymbolNotFound(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	repo := &fakeCandleRepo{series: map[domain.Symbol]domain.CandleSeries{}}
	universe := &fakeSymbolUniverse{syms: []domain.Symbol{}}

	uc := usecases.NewGetSymbolDetail(repo, nil, universe, "x", "y", 200, 1000)
	_, err := uc.Execute(context.Background(), usecases.GetSymbolDetailRequest{
		Symbol:    sym,
		Timeframe: tf,
		Limit:     200,
	})
	if err == nil || !errors.Is(err, usecases.ErrSymbolNotFound) {
		t.Fatalf("expected ErrSymbolNotFound, got %v", err)
	}
}

func TestGetSymbolDetailInvalidLimitDefaults(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	series := mustSeries(sym, tf, 1)
	repo := &fakeCandleRepo{series: map[domain.Symbol]domain.CandleSeries{sym: series}}
	universe := &fakeSymbolUniverse{syms: []domain.Symbol{sym}}

	uc := usecases.NewGetSymbolDetail(repo, nil, universe, "x", "y", 2, 5)
	_, err := uc.Execute(context.Background(), usecases.GetSymbolDetailRequest{
		Symbol:    sym,
		Timeframe: tf,
		Limit:     0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastN != 2 {
		t.Fatalf("expected limit 2, got %d", repo.lastN)
	}
}

func TestGetSymbolDetailClampsLimit(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	series := mustSeries(sym, tf, 1)
	repo := &fakeCandleRepo{series: map[domain.Symbol]domain.CandleSeries{sym: series}}
	universe := &fakeSymbolUniverse{syms: []domain.Symbol{sym}}

	uc := usecases.NewGetSymbolDetail(repo, nil, universe, "x", "y", 2, 3)
	_, err := uc.Execute(context.Background(), usecases.GetSymbolDetailRequest{
		Symbol:    sym,
		Timeframe: tf,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastN != 3 {
		t.Fatalf("expected limit 3, got %d", repo.lastN)
	}
}

func TestGetSymbolDetailNoCandlesReturnsEmpty(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("1h")
	emptySeries, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{})
	repo := &fakeCandleRepo{series: map[domain.Symbol]domain.CandleSeries{sym: emptySeries}}
	universe := &fakeSymbolUniverse{syms: []domain.Symbol{sym}}
	scorer := &fakeScorer{stats: usecases.SymbolStats{TotalScore: 0.0, Scores: map[string]float64{}}}

	uc := usecases.NewGetSymbolDetail(repo, scorer, universe, "x", "y", 2, 3)
	result, err := uc.Execute(context.Background(), usecases.GetSymbolDetailRequest{
		Symbol:    sym,
		Timeframe: tf,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Candles) != 0 {
		t.Fatalf("expected 0 candles, got %d", len(result.Candles))
	}
	if result.Stats == nil {
		t.Fatalf("expected stats, got nil")
	}
}
