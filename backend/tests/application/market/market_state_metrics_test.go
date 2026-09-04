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

func (f *fakeMetricsCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
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

// TestMarketStateService_Calculate_SkipsCandleFanoutEvenWithProvider is the
// regression test for the PR-073 CR finding that /api/market/state (and the
// notification scheduler, and the setup scanner) got significantly more
// expensive because every Calculate() call paid for a full symbol-universe
// candle fan-out, whether or not the caller read VolatilityExpansion/
// Dispersion. Calculate must stay cheap even when a CandleProvider is
// configured for CalculateWithCandleMetrics's benefit — only the latter may
// touch it.
func TestMarketStateService_Calculate_SkipsCandleFanoutEvenWithProvider(t *testing.T) {
	spy := &fanoutSpyCandleProvider{}
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})
	svc.SetCandleProvider(spy)

	s, err := svc.Calculate("4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.symbolsCalls != 0 {
		t.Errorf("expected Calculate to never touch the CandleProvider, got %d Symbols() calls", spy.symbolsCalls)
	}
	if s.VolatilityExpansion != 1.0 || s.Dispersion != 0 {
		t.Errorf("expected default metrics from Calculate, got vol=%f disp=%f", s.VolatilityExpansion, s.Dispersion)
	}
}

type fanoutSpyCandleProvider struct {
	symbolsCalls int
}

func (f *fanoutSpyCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	f.symbolsCalls++
	return nil, nil
}

func (f *fanoutSpyCandleProvider) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	return domain.CandleSeries{}, fmt.Errorf("should never be called by Calculate")
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

	s, err := svc.CalculateWithCandleMetrics(context.Background(), "4h")
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

	s, err := svc.CalculateWithCandleMetrics(context.Background(), "4h")
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

// --- Cancellation ---

// stuckCandleProvider's GetLastNCandles blocks until the test process exits
// — simulating a provider that ignores the context it's handed (a stalled
// network call whose driver doesn't honor cancellation, or simply a buggy
// implementation). GetLastNCandles now takes a context.Context, and
// well-behaved implementations (e.g. FreeTierCandleRepository) abort their
// underlying HTTP request when it's cancelled, but fetchCandleResults must
// not assume every implementation does — this fake deliberately discards
// ctx to prove the racing-select safety net still protects the caller even
// then.
type stuckCandleProvider struct {
	symbols []domain.Symbol
}

func (p *stuckCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return p.symbols, nil
}

func (p *stuckCandleProvider) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	select {} // block forever, ignoring ctx
}

// TestMarketStateService_CalculateWithCandleMetrics_CancelledContextReturnsPromptly
// covers the CR finding that a cancelled request context left the caller
// blocked through in-flight candle fetches. stuckCandleProvider ignores the
// ctx it's given, so it genuinely can't be aborted mid-call, but the caller
// must not wait for it regardless — it should get its answer (defaults,
// since no real result exists) as soon as ctx is done, not after the stuck
// fetch eventually completes (which in this test is never).
func TestMarketStateService_CalculateWithCandleMetrics_CancelledContextReturnsPromptly(t *testing.T) {
	sym := makeSymbol2("BTCUSDT")
	svc := appmarket.NewMarketStateService(&fakeEvalProvider{evals: []domain.EvaluationSnapshot{oneTrendEval()}})
	svc.SetCandleProvider(&stuckCandleProvider{symbols: []domain.Symbol{sym}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	var s mkt.Summary
	var err error
	go func() {
		s, err = svc.CalculateWithCandleMetrics(ctx, "4h")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CalculateWithCandleMetrics did not return promptly after context cancellation")
	}

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.VolatilityExpansion != 1.0 {
		t.Errorf("expected default VolatilityExpansion on cancellation, got %f", s.VolatilityExpansion)
	}
}
