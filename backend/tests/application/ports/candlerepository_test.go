package ports_test

import (
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// FakeCandleRepository is a stub implementation for testing the port contract.
type FakeCandleRepository struct {
	series    domain.CandleSeries
	shouldErr bool
}

// GetSeries implements CandleRepositoryPort.
func (f *FakeCandleRepository) GetSeries(
	symbol domain.Symbol,
	timeframe domain.Timeframe,
	from time.Time,
	to time.Time,
) (domain.CandleSeries, error) {
	if f.shouldErr {
		return domain.CandleSeries{}, errors.New("fake error")
	}
	return f.series, nil
}

func TestCandleRepositoryPort_DefinesGetSeriesMethod(t *testing.T) {
	// Verify the port interface can be implemented
	_ = ports.CandleRepositoryPort((&FakeCandleRepository{}))
}

func TestCandleRepositoryPort_GetSeriesReturnsCandleSeries(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts.Add(1*time.Minute), 200, 210, 190, 205, 2000),
	}

	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("failed to create test series: %v", err)
	}

	repo := &FakeCandleRepository{series: series}

	result, err := repo.GetSeries(sym, tf, ts, ts.Add(5*time.Minute))

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.Len() != 2 {
		t.Errorf("expected 2 candles, got %d", result.Len())
	}
}

func TestCandleRepositoryPort_GetSeriesAllowsEmptyResult(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create empty series
	series, err := domain.NewCandleSeries(sym, tf, []domain.Candle{})
	if err != nil {
		t.Fatalf("failed to create empty series: %v", err)
	}

	repo := &FakeCandleRepository{series: series}

	result, err := repo.GetSeries(sym, tf, ts, ts.Add(5*time.Minute))

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.Len() != 0 {
		t.Errorf("expected 0 candles, got %d", result.Len())
	}
}

func TestCandleRepositoryPort_ReturnsErrorOnFailure(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	repo := &FakeCandleRepository{shouldErr: true}

	_, err := repo.GetSeries(sym, tf, ts, ts.Add(5*time.Minute))

	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCandleRepositoryPort_AcceptsTimeRange(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts, 100, 110, 90, 105, 1000),
	}

	series, _ := domain.NewCandleSeries(sym, tf, candles)
	repo := &FakeCandleRepository{series: series}

	// Port should accept time range parameters
	result, err := repo.GetSeries(
		sym,
		tf,
		ts,
		ts.Add(10*time.Minute),
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.Len() == 0 {
		t.Error("expected result to contain candles")
	}
}
