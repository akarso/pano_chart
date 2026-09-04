package market_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// --- Fake CandleProvider (full OHLC, needed for real ATR/volatility math —
// composite_index_test.go's fakeCandleProvider flattens high==low==close,
// which always yields ATR==0). ---

type fakeMetricsCandle struct {
	symbol domain.Symbol
	tf     domain.Timeframe
	ts     time.Time
	open   float64
	high   float64
	low    float64
	close  float64
}

type fakeMetricsCandleProvider struct {
	symbols []domain.Symbol
	candles map[string][]fakeMetricsCandle
	err     error
}

func (f *fakeMetricsCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}

func (f *fakeMetricsCandleProvider) GetLastNCandles(sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	fcs, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("no data for %s", key)
	}
	candles := make([]domain.Candle, 0, len(fcs))
	for _, fc := range fcs {
		candles = append(candles, domain.NewCandleUnsafe(fc.symbol, fc.tf, fc.ts, fc.open, fc.high, fc.low, fc.close, 1000))
	}
	if n < len(candles) {
		candles = candles[len(candles)-n:]
	}
	return domain.NewCandleSeries(sym, tf, candles)
}

func metricsTS(idx int) time.Time {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(idx) * 4 * time.Hour)
}

// oneTrendEval is a minimal evaluation snapshot that makes trend dominant
// without triggering health dampening (ATR left at 0 — see
// MarketStateService.Calculate's "skip tokens without price data" guard).
func oneTrendEval() domain.EvaluationSnapshot {
	return domain.EvaluationSnapshot{TrendScore: 0.9, SidewaysScore: 0.1, CompressionScore: 0.05}
}

func TestMarketStateService_NoCandleProvider_DefaultsMetrics(t *testing.T) {
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})

	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.VolatilityExpansion != 1.0 {
		t.Errorf("expected default VolatilityExpansion 1.0 without a CandleProvider, got %f", s.VolatilityExpansion)
	}
	if s.Dispersion != 0 {
		t.Errorf("expected default Dispersion 0 without a CandleProvider, got %f", s.Dispersion)
	}
}

func TestMarketStateService_VolatilityExpansion_InsufficientData(t *testing.T) {
	// < 30 candles → volatility defaults to 1.0 (same rule as the deleted
	// softmax pipeline's TestMetrics_VolatilityExpansion_InsufficientData).
	sym := makeSymbol2("BTCUSDT")
	tf := makeTimeframe2("4h")

	candles := make([]fakeMetricsCandle, 5)
	for i := 0; i < 5; i++ {
		candles[i] = fakeMetricsCandle{symbol: sym, tf: tf, ts: metricsTS(i), open: 50000, high: 50010, low: 49990, close: 50000}
	}

	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})
	svc.SetCandleProvider(&fakeMetricsCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]fakeMetricsCandle{"BTCUSDT:4h": candles},
	})

	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.VolatilityExpansion != 1.0 {
		t.Errorf("expected VolatilityExpansion 1.0 for insufficient data, got %f", s.VolatilityExpansion)
	}
}

func TestMarketStateService_Dispersion_DivergentReturns(t *testing.T) {
	// Two symbols with strongly divergent period returns → positive dispersion.
	sym1 := makeSymbol2("BTCUSDT")
	sym2 := makeSymbol2("ETHUSDT")
	tf := makeTimeframe2("4h")

	btc := make([]fakeMetricsCandle, 110)
	eth := make([]fakeMetricsCandle, 110)
	for i := 0; i < 110; i++ {
		bp := 50000 + float64(i)*500 // BTC up strongly
		ep := 3000 - float64(i)*20   // ETH down
		btc[i] = fakeMetricsCandle{symbol: sym1, tf: tf, ts: metricsTS(i), open: bp, high: bp + 10, low: bp - 10, close: bp}
		eth[i] = fakeMetricsCandle{symbol: sym2, tf: tf, ts: metricsTS(i), open: ep, high: ep + 10, low: ep - 10, close: ep}
	}

	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})
	svc.SetCandleProvider(&fakeMetricsCandleProvider{
		symbols: []domain.Symbol{sym1, sym2},
		candles: map[string][]fakeMetricsCandle{"BTCUSDT:4h": btc, "ETHUSDT:4h": eth},
	})

	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Dispersion <= 0 {
		t.Errorf("expected positive dispersion for divergent assets, got %f", s.Dispersion)
	}
}

// --- RegimeObserver notification ---

type fakeRegimeObserver struct {
	calls []mkt.Regime
}

func (f *fakeRegimeObserver) Update(_ string, regime mkt.Regime, _ int64) error {
	f.calls = append(f.calls, regime)
	return nil
}

func TestMarketStateService_NotifiesObserver(t *testing.T) {
	obs := &fakeRegimeObserver{}
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})
	svc.SetObserver(obs)

	if _, err := svc.Calculate("4h"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs.calls) != 1 {
		t.Fatalf("expected 1 observer call, got %d", len(obs.calls))
	}
	if obs.calls[0] != mkt.RegimeTrend {
		t.Errorf("expected observer notified of trend, got %s", obs.calls[0])
	}
}
