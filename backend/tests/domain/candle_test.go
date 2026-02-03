package domain_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/akarso/pano_chart/backend/domain"
)

func mustTF(t *testing.T, s string) domain.Timeframe {
	tf, err := domain.NewTimeframe(s)
	if err != nil {
		t.Fatalf("failed to create timeframe %q: %v", s, err)
	}
	return tf
}

func mustSym(t *testing.T, s string) domain.Symbol {
	sym, err := domain.NewSymbol(s)
	if err != nil {
		t.Fatalf("failed to create symbol %q: %v", s, err)
	}
	return sym
}

func TestCandle_CreatesWithValidData(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	opent := time.Date(2026, 2, 3, 12, 34, 0, 0, time.UTC) // second=0
	candle, err := domain.NewCandle(sym, tf, opent, 100.0, 105.0, 99.0, 102.0, 1234.5)
	if err != nil {
		t.Fatalf("NewCandle returned error: %v", err)
	}
	if candle.Symbol() != sym {
		t.Fatalf("expected symbol %v, got %v", sym, candle.Symbol())
	}
	if candle.Timeframe() != tf {
		t.Fatalf("expected timeframe %v, got %v", tf, candle.Timeframe())
	}
	if !candle.Timestamp().Equal(opent) {
		t.Fatalf("expected timestamp %v, got %v", opent, candle.Timestamp())
	}
}

func TestCandle_RejectsNegativePrices(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	opent := time.Date(2026, 2, 3, 12, 30, 0, 0, time.UTC)
	cases := []struct {
		o float64
		h float64
		l float64
		c float64
		v float64
	}{
		{-1, 10, 0, 5, 1},
		{1, -10, 0, 5, 1},
		{1, 10, -5, 5, 1},
		{1, 10, 0, -2, 1},
		{1, 10, 0, 5, -1},
	}
	for i, cc := range cases {
		name := fmt.Sprintf("case_%d", i)
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewCandle(sym, tf, opent, cc.o, cc.h, cc.l, cc.c, cc.v)
			if err == nil {
				t.Fatalf("expected error for negative values in case %d", i)
			}
		})
	}
}

func TestCandle_RejectsInvalidHighLowInvariant(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	opent := time.Date(2026, 2, 3, 12, 30, 0, 0, time.UTC)
	// High < max(open,close)
	_, err := domain.NewCandle(sym, tf, opent, 100, 99, 95, 98, 1)
	if err == nil { t.Fatalf("expected error when High < max(Open,Close)") }
	// Low > min(open,close)
	_, err = domain.NewCandle(sym, tf, opent, 100, 110, 101, 105, 1)
	if err == nil { t.Fatalf("expected error when Low > min(Open,Close)") }
	// High < Low
	_, err = domain.NewCandle(sym, tf, opent, 100, 100, 101, 100, 1)
	if err == nil { t.Fatalf("expected error when High < Low") }
}

func TestCandle_RejectsNonUTCTimestamp(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	loc, _ := time.LoadLocation("America/New_York")
	opent := time.Date(2026, 2, 3, 12, 30, 0, 0, loc)
	_, err := domain.NewCandle(sym, tf, opent, 1,2,0,1,1)
	if err == nil { t.Fatalf("expected error for non-UTC timestamp") }
}

func TestCandle_RejectsMisalignedTimestamp(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf1 := mustTF(t, "1m")
	tf5 := mustTF(t, "5m")
	// 1m candle but second != 0
	opent := time.Date(2026, 2, 3, 12, 30, 5, 0, time.UTC)
	_, err := domain.NewCandle(sym, tf1, opent, 1,2,0,1,1)
	if err == nil { t.Fatalf("expected error for misaligned second for 1m") }
	// 5m candle but minute not divisible by 5
	opent2 := time.Date(2026, 2, 3, 12, 32, 0, 0, time.UTC)
	_, err = domain.NewCandle(sym, tf5, opent2, 1,2,0,1,1)
	if err == nil { t.Fatalf("expected error for misaligned minute for 5m") }
}

func TestCandle_EqualityBasedOnIdentity(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	opent := time.Date(2026, 2, 3, 12, 30, 0, 0, time.UTC)
	c1, _ := domain.NewCandle(sym, tf, opent, 1,2,0,1,1)
	c2, _ := domain.NewCandle(sym, tf, opent, 10,20,5,15,100)
	if !c1.Equals(c2) {
		t.Fatalf("candles with same symbol,timeframe,timestamp should be equal")
	}
	// different timestamp
	c3, _ := domain.NewCandle(sym, tf, time.Date(2026,2,3,12,31,0,0,time.UTC),1,2,0,1,1)
	if c1.Equals(c3) { t.Fatalf("candles with different timestamp should not be equal") }
}

func TestCandle_BullBearDojiClassification(t *testing.T) {
	sym := mustSym(t, "BTC_USDT")
	tf := mustTF(t, "1m")
	opent := time.Date(2026,2,3,12,30,0,0,time.UTC)
	b, _ := domain.NewCandle(sym, tf, opent, 1,2,0,2,1)
	if !b.IsBullish() { t.Fatalf("expected bullish") }
	if b.IsBearish() { t.Fatalf("not bearish") }
	if b.IsDoji() { t.Fatalf("not doji") }
	b2, _ := domain.NewCandle(sym, tf, opent, 2,3,1,1,1)
	if !b2.IsBearish() { t.Fatalf("expected bearish") }
	if b2.IsBullish() { t.Fatalf("not bullish") }
	if b2.IsDoji() { t.Fatalf("not doji") }
	b3, _ := domain.NewCandle(sym, tf, opent, 1,1,1,1,1)
	if !b3.IsDoji() { t.Fatalf("expected doji") }
}
