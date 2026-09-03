package market_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

// ---------- Fakes ----------

type backfillCandleProvider struct {
	symbols []domain.Symbol
	// candles keyed by "SYMBOL:TF"
	candles map[string][]domain.Candle
}

func (f *backfillCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	return f.symbols, nil
}

func (f *backfillCandleProvider) GetLastNCandles(sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	key := sym.String() + ":" + tf.String()
	cs, ok := f.candles[key]
	if !ok {
		return domain.CandleSeries{}, fmt.Errorf("no data for %s", key)
	}
	if n < len(cs) {
		cs = cs[len(cs)-n:]
	}
	return domain.NewCandleSeries(sym, tf, cs)
}

type observedRegime struct {
	Timeframe string
	Regime    mkt.Regime
	Timestamp int64
}

type fakeObserver struct {
	calls []observedRegime
}

func (f *fakeObserver) Update(timeframe string, regime mkt.Regime, ts int64) error {
	f.calls = append(f.calls, observedRegime{timeframe, regime, ts})
	return nil
}

// ---------- Helpers ----------

func bfSym(s string) domain.Symbol {
	sym, _ := domain.NewSymbol(s)
	return sym
}

func bfTF(s string) domain.Timeframe {
	tf, _ := domain.NewTimeframe(s)
	return tf
}

func bfTS(idx int) time.Time {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(idx) * 4 * time.Hour)
}

// makeStableCandles builds `n` flat candles with small oscillation around
// basePrice.  shortATR ≈ longATR → volatility ~1.0, sideways-like.
func makeStableCandles(sym domain.Symbol, tf domain.Timeframe, n int, basePrice float64) []domain.Candle {
	out := make([]domain.Candle, n)
	for i := 0; i < n; i++ {
		p := basePrice + float64(i)*0.01
		out[i] = domain.NewCandleUnsafe(sym, tf, bfTS(i), p, p+5, p-5, p, 1000)
	}
	return out
}

// ---------- Tests ----------

func TestBackfiller_PopulatesHistory(t *testing.T) {
	sym := bfSym("BTCUSDT")
	tf := bfTF("4h")

	// 150 stable candles → 41 windowable steps.  Ask for 20.
	candles := makeStableCandles(sym, tf, 150, 50000)

	provider := &backfillCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:4h": candles},
	}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 20); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) != 20 {
		t.Fatalf("expected 20 observations, got %d", len(obs.calls))
	}

	// Timestamps must be strictly increasing (oldest first).
	for i := 1; i < len(obs.calls); i++ {
		if obs.calls[i].Timestamp <= obs.calls[i-1].Timestamp {
			t.Errorf("timestamps not ascending at index %d: %d <= %d",
				i, obs.calls[i].Timestamp, obs.calls[i-1].Timestamp)
		}
	}

	// All observations should reference the requested timeframe.
	for i, o := range obs.calls {
		if o.Timeframe != "4h" {
			t.Errorf("call %d: timeframe %q, want 4h", i, o.Timeframe)
		}
	}
}

func TestBackfiller_NoSymbolsNoop(t *testing.T) {
	provider := &backfillCandleProvider{symbols: nil}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) != 0 {
		t.Errorf("expected 0 observations, got %d", len(obs.calls))
	}
}

func TestBackfiller_InsufficientCandlesNoop(t *testing.T) {
	sym := bfSym("BTCUSDT")
	tf := bfTF("4h")

	// Only 10 candles — not enough for a 110-candle window.
	candles := makeStableCandles(sym, tf, 10, 50000)

	provider := &backfillCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:4h": candles},
	}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) != 0 {
		t.Errorf("expected 0 observations, got %d", len(obs.calls))
	}
}

func TestBackfiller_StepsClampedByData(t *testing.T) {
	sym := bfSym("BTCUSDT")
	tf := bfTF("4h")

	// 115 candles → max 6 windows (115-110+1).  Ask for 100 steps.
	candles := makeStableCandles(sym, tf, 115, 50000)

	provider := &backfillCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:4h": candles},
	}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) != 6 {
		t.Errorf("expected 6 observations (clamped), got %d", len(obs.calls))
	}
}

func TestBackfiller_MultipleSymbols(t *testing.T) {
	sym1 := bfSym("BTCUSDT")
	sym2 := bfSym("ETHUSDT")
	tf := bfTF("4h")

	candles1 := makeStableCandles(sym1, tf, 150, 50000)
	candles2 := makeStableCandles(sym2, tf, 150, 3000)

	provider := &backfillCandleProvider{
		symbols: []domain.Symbol{sym1, sym2},
		candles: map[string][]domain.Candle{
			"BTCUSDT:4h": candles1,
			"ETHUSDT:4h": candles2,
		},
	}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 20); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) != 20 {
		t.Fatalf("expected 20 observations, got %d", len(obs.calls))
	}
}

func TestBackfiller_InvalidTimeframe(t *testing.T) {
	provider := &backfillCandleProvider{symbols: []domain.Symbol{bfSym("BTCUSDT")}}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	err := bf.Run(context.Background(), "invalid", 10)
	if err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestBackfiller_DetectsRegimeVariation(t *testing.T) {
	// Build candle data that transitions from a strong, clean uptrend to a
	// suddenly flat / tight range so the backfiller's sliding windows
	// produce at least two different regimes (e.g. trend vs sideways).
	sym := bfSym("BTCUSDT")
	tf := bfTF("4h")

	n := 200
	candles := make([]domain.Candle, n)

	// First 60%: consistent uptrend — close rises steadily, moderate range.
	boundary := n * 6 / 10
	for i := 0; i < boundary; i++ {
		p := 50000.0 + float64(i)*2.0 // +2 per candle → strong consistent trend
		rangeSize := 5.0              // stable moderate wicks
		candles[i] = domain.NewCandleUnsafe(sym, tf, bfTS(i), p, p+rangeSize, p-rangeSize, p, 1000)
	}
	// Second 40%: oscillate around the last trending close — genuine
	// sideways behavior that the scoring calculators recognise clearly.
	flatClose := 50000.0 + float64(boundary-1)*2.0
	for i := boundary; i < n; i++ {
		offset := 8.0 * math.Sin(float64(i)*0.5)
		c := flatClose + offset
		candles[i] = domain.NewCandleUnsafe(sym, tf, bfTS(i), c, c+3, c-3, c, 1000)
	}

	provider := &backfillCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:4h": candles},
	}
	obs := &fakeObserver{}

	bf := metrics.NewBackfiller(provider, obs)
	if err := bf.Run(context.Background(), "4h", 60); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(obs.calls) == 0 {
		t.Fatal("expected at least one observation")
	}

	// Collect distinct regimes.
	regimes := make(map[mkt.Regime]bool)
	for _, o := range obs.calls {
		regimes[o.Regime] = true
	}

	// When windows slide from wide-range territory into tight-range territory
	// the detected regime should vary (e.g. sideways vs. compression).
	if len(regimes) < 2 {
		t.Errorf("expected at least 2 distinct regimes, got %v", regimes)
	}
}
