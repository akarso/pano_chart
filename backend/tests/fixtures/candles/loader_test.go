package fixtures_test

import (
	"testing"
	"time"

	"pano_chart/backend/domain"
	"pano_chart/backend/tests/fixtures/candles"
)

func TestLoad_ValidFixture(t *testing.T) {
	series := fixtures.Load(t, "clean_uptrend")
	if series.Len() != fixtures.RequiredBars {
		t.Fatalf("len=%d want %d", series.Len(), fixtures.RequiredBars)
	}
	c, err := series.At(0)
	if err != nil {
		t.Fatal(err)
	}
	if c.High() < c.Open() || c.High() < c.Close() || c.Low() > c.Open() || c.Low() > c.Close() {
		t.Fatalf("loaded candle violates OHLC invariants: o=%g h=%g l=%g c=%g",
			c.Open(), c.High(), c.Low(), c.Close())
	}
}

func TestLoad_Contract_NewCandleRejectsBadOHLC(t *testing.T) {
	// Load uses domain.NewCandle (not Unsafe). Confirm the validation gate
	// that protects regenerated fixtures.
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.NewTimeframeUnsafe("15m")
	ts := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := domain.NewCandle(sym, tf, ts, 100, 90, 95, 100, 1) // high < open
	if err == nil {
		t.Fatal("expected NewCandle to reject high < open")
	}
}
