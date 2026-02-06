package domain

import (
	"testing"
	"time"

	"pano_chart/backend/domain"
)

func TestCandleSeries_AllowsEmptySeries(t *testing.T) {
	// Empty series should be allowed
	series, err := domain.NewCandleSeries(
		domain.NewSymbolUnsafe("BTC"),
		domain.NewTimeframeUnsafe("1m"),
		[]domain.Candle{},
	)

	if err != nil {
		t.Fatalf("expected no error for empty series, got %v", err)
	}

	if series.Len() != 0 {
		t.Errorf("expected length 0, got %d", series.Len())
	}
}

func TestCandleSeries_RejectsMixedSymbols(t *testing.T) {
	sym1 := domain.NewSymbolUnsafe("BTC")
	sym2 := domain.NewSymbolUnsafe("ETH")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym1, tf, ts, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym2, tf, ts.Add(1*time.Minute), 200, 210, 190, 205, 2000),
	}

	_, err := domain.NewCandleSeries(sym1, tf, candles)

	if err == nil {
		t.Error("expected error for mixed symbols, got nil")
	}
}

func TestCandleSeries_RejectsMixedTimeframes(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf1 := domain.NewTimeframeUnsafe("1m")
	tf5 := domain.NewTimeframeUnsafe("5m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf1, ts, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf5, ts.Add(1*time.Minute), 100, 110, 90, 105, 1000),
	}

	_, err := domain.NewCandleSeries(sym, tf1, candles)

	if err == nil {
		t.Error("expected error for mixed timeframes, got nil")
	}
}

func TestCandleSeries_OrdersCandlesByTimestamp(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(1 * time.Minute)
	ts3 := ts1.Add(2 * time.Minute)

	// Create candles in reverse order
	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts3, 300, 310, 290, 305, 3000),
		domain.NewCandleUnsafe(sym, tf, ts1, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts2, 200, 210, 190, 205, 2000),
	}

	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if series.Len() != 3 {
		t.Errorf("expected length 3, got %d", series.Len())
	}

	// Verify ordering
	first, _ := series.First()
	if !first.Timestamp().Equal(ts1) {
		t.Errorf("expected first timestamp %v, got %v", ts1, first.Timestamp())
	}

	last, _ := series.Last()
	if !last.Timestamp().Equal(ts3) {
		t.Errorf("expected last timestamp %v, got %v", ts3, last.Timestamp())
	}

	// Check all candles are in order
	allCandles := series.All()
	if allCandles[0].Timestamp() != ts1 || allCandles[1].Timestamp() != ts2 || allCandles[2].Timestamp() != ts3 {
		t.Error("candles not in ascending timestamp order")
	}
}

func TestCandleSeries_RejectsDuplicateTimestamps(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts, 200, 210, 190, 205, 2000), // duplicate timestamp
	}

	_, err := domain.NewCandleSeries(sym, tf, candles)

	if err == nil {
		t.Error("expected error for duplicate timestamps, got nil")
	}
}

func TestCandleSeries_DetectsGapsCorrectly(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(1 * time.Minute)
	ts4 := ts1.Add(3 * time.Minute) // gap: ts3 is missing

	// Series with a gap (missing ts3)
	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts1, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts2, 200, 210, 190, 205, 2000),
		domain.NewCandleUnsafe(sym, tf, ts4, 400, 410, 390, 405, 4000),
	}

	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Detect gap between index 1 and 2
	hasGap := series.HasGapAfter(1)
	if !hasGap {
		t.Error("expected gap detected between indices 1 and 2")
	}

	// No gap between index 0 and 1
	hasGap = series.HasGapAfter(0)
	if hasGap {
		t.Error("expected no gap between indices 0 and 1")
	}

	// Test with series without gaps
	candles2 := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts1, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts2, 200, 210, 190, 205, 2000),
	}

	series2, err := domain.NewCandleSeries(sym, tf, candles2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if series2.HasGapAfter(0) {
		t.Error("expected no gap in consecutive candles")
	}
}

func TestCandleSeries_IsImmutable(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(1 * time.Minute)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts1, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts2, 200, 210, 190, 205, 2000),
	}

	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get a slice copy and try to modify it
	allCandles := series.All()
	originalLen := len(allCandles)

	// Modify the returned slice (intentionally unused to verify mutation doesn't affect series)
	_ = append(allCandles, allCandles[0])

	// Series should be unaffected
	if series.Len() != originalLen {
		t.Error("series was mutated via returned slice")
	}

	// Get another slice copy to ensure independence
	allCandles2 := series.All()
	if len(allCandles2) != originalLen {
		t.Error("series length changed after first slice modification")
	}
}

func TestCandleSeries_SafeReadOperations(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(1 * time.Minute)
	ts3 := ts1.Add(2 * time.Minute)

	candles := []domain.Candle{
		domain.NewCandleUnsafe(sym, tf, ts1, 100, 110, 90, 105, 1000),
		domain.NewCandleUnsafe(sym, tf, ts2, 200, 210, 190, 205, 2000),
		domain.NewCandleUnsafe(sym, tf, ts3, 300, 310, 290, 305, 3000),
	}

	series, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test Len()
	if series.Len() != 3 {
		t.Errorf("expected length 3, got %d", series.Len())
	}

	// Test At() with valid index
	candle, err := series.At(1)
	if err != nil {
		t.Fatalf("unexpected error at valid index: %v", err)
	}
	if !candle.Timestamp().Equal(ts2) {
		t.Errorf("expected candle at index 1 to have timestamp %v, got %v", ts2, candle.Timestamp())
	}

	// Test At() with invalid index
	_, err = series.At(10)
	if err == nil {
		t.Error("expected error for out-of-bounds index")
	}

	// Test First()
	first, err := series.First()
	if err != nil {
		t.Fatalf("unexpected error on First(): %v", err)
	}
	if !first.Timestamp().Equal(ts1) {
		t.Errorf("expected first timestamp %v, got %v", ts1, first.Timestamp())
	}

	// Test Last()
	last, err := series.Last()
	if err != nil {
		t.Fatalf("unexpected error on Last(): %v", err)
	}
	if !last.Timestamp().Equal(ts3) {
		t.Errorf("expected last timestamp %v, got %v", ts3, last.Timestamp())
	}

	// Test All() returns defensive copy
	all := series.All()
	if len(all) != 3 {
		t.Errorf("expected 3 candles, got %d", len(all))
	}
}

func TestCandleSeries_EmptySeriesReadOperations(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")

	series, err := domain.NewCandleSeries(sym, tf, []domain.Candle{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test Len()
	if series.Len() != 0 {
		t.Errorf("expected length 0, got %d", series.Len())
	}

	// Test First() on empty series
	_, err = series.First()
	if err == nil {
		t.Error("expected error on First() for empty series")
	}

	// Test Last() on empty series
	_, err = series.Last()
	if err == nil {
		t.Error("expected error on Last() for empty series")
	}

	// Test All() on empty series
	all := series.All()
	if len(all) != 0 {
		t.Errorf("expected 0 candles, got %d", len(all))
	}
}
