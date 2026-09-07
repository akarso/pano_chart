package risk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/risk"
	"pano_chart/backend/domain"
)

// --- Fakes ---

type fakeFuturesPort struct {
	funding   float64
	oi        []float64
	longRatio float64

	fundingErr   error
	oiErr        error
	longRatioErr error

	fundingCalls   int
	oiCalls        int
	longRatioCalls int
}

func (f *fakeFuturesPort) FundingRate(_ context.Context, _ string) (float64, error) {
	f.fundingCalls++
	if f.fundingErr != nil {
		return 0, f.fundingErr
	}
	return f.funding, nil
}

func (f *fakeFuturesPort) OpenInterestHistory(_ context.Context, _ string) ([]float64, error) {
	f.oiCalls++
	if f.oiErr != nil {
		return nil, f.oiErr
	}
	return f.oi, nil
}

func (f *fakeFuturesPort) LongShortRatio(_ context.Context, _ string) (float64, error) {
	f.longRatioCalls++
	if f.longRatioErr != nil {
		return 0, f.longRatioErr
	}
	return f.longRatio, nil
}

type fakeCandleRepo struct {
	series domain.CandleSeries
	err    error
}

func (f *fakeCandleRepo) GetSeries(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _, _ time.Time) (domain.CandleSeries, error) {
	return f.series, f.err
}

func (f *fakeCandleRepo) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

// makeCandleSeries builds a simple n-candle 4h series with a fixed close
// (100) and a high/low band, so Price and the nearest-cluster proxy are
// both predictable in tests.
func makeCandleSeries(t *testing.T, n int) domain.CandleSeries {
	t.Helper()
	sym, err := domain.NewSymbol("BTCUSDT")
	if err != nil {
		t.Fatalf("NewSymbol: %v", err)
	}
	tf, err := domain.NewTimeframe("4h")
	if err != nil {
		t.Fatalf("NewTimeframe: %v", err)
	}
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			100, 110, 90, 100, 1000,
		)
	}
	s, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("NewCandleSeries: %v", err)
	}
	return s
}

// --- Tests ---

func TestBinanceFuturesDataProvider_ImplementsDataProvider(t *testing.T) {
	var _ risk.DataProvider = risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
}

func TestBinanceFuturesDataProvider_HappyPath_ComposesRealFuturesDataWithCandleDerivedPrice(t *testing.T) {
	series := makeCandleSeries(t, 50)
	futures := &fakeFuturesPort{
		funding:   0.0002,
		oi:        []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 200},
		longRatio: 0.62,
	}
	repo := &fakeCandleRepo{series: series}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	data, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Funding != 0.0002 {
		t.Errorf("expected Funding 0.0002 (from futures port), got %v", data.Funding)
	}
	if len(data.OISeries) != 10 {
		t.Fatalf("expected 10 OI entries, got %d", len(data.OISeries))
	}
	if data.LongRatio != 0.62 {
		t.Errorf("expected LongRatio 0.62 (from futures port), got %v", data.LongRatio)
	}
	if data.Price != 100 {
		t.Errorf("expected Price 100 (from last candle close), got %v", data.Price)
	}
	if data.NearestCluster == 0 {
		t.Errorf("expected a nonzero nearest-cluster proxy from candle data")
	}
}

func TestBinanceFuturesDataProvider_FundingRateError_FailsFastWithoutCallingOtherEndpoints(t *testing.T) {
	// Regression test for PR-081 §5: a futures-data failure must fail the
	// whole call (SetupService already degrades Crowding to its zero
	// default on a non-cancellation FragilityProvider error) rather than
	// silently proceeding with partial/zero data. Also confirms FundingRate
	// failing short-circuits before the other two real calls are made.
	futures := &fakeFuturesPort{fundingErr: errors.New("no futures market for symbol")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "NOTASYMBOL", "4h")
	if err == nil {
		t.Fatal("expected error when FundingRate fails")
	}
	if futures.oiCalls != 0 || futures.longRatioCalls != 0 {
		t.Errorf("expected no further futures calls after FundingRate failed, got oiCalls=%d longRatioCalls=%d",
			futures.oiCalls, futures.longRatioCalls)
	}
}

func TestBinanceFuturesDataProvider_OpenInterestHistoryError_FailsCall(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oiErr: errors.New("no data")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when OpenInterestHistory fails")
	}
	if futures.longRatioCalls != 0 {
		t.Errorf("expected LongShortRatio not to be called after OpenInterestHistory failed")
	}
}

func TestBinanceFuturesDataProvider_LongShortRatioError_FailsCall(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatioErr: errors.New("no data")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when LongShortRatio fails")
	}
}

func TestBinanceFuturesDataProvider_InvalidSymbol(t *testing.T) {
	p := risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
	_, err := p.Get(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for invalid symbol")
	}
}

func TestBinanceFuturesDataProvider_InvalidTimeframe(t *testing.T) {
	p := risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
	_, err := p.Get(context.Background(), "BTCUSDT", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestBinanceFuturesDataProvider_CandleFetchError(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	repo := &fakeCandleRepo{err: errors.New("network failure")}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when candle fetch fails")
	}
}

func TestBinanceFuturesDataProvider_InsufficientCandleData(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 1)} // fewer than 2 candles
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error for insufficient candle data")
	}
}
