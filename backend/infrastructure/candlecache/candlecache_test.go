package infrastructure

import (
	"testing"
	"time"
)

type testCandle struct{ v int }

func TestCandleCache_TTL(t *testing.T) {
	cache := NewCandleCache()
	key := CandleCacheKey{Symbol: "BTCUSDT", Timeframe: "5m"}
	candles := []testCandle{{1}, {2}, {3}}

	cache.Set(key, candles, 50*time.Millisecond)
	if got, ok := cache.Get(key); !ok || len(got.([]testCandle)) != 3 {
		t.Errorf("expected cache hit, got %v", got)
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := cache.Get(key); ok {
		t.Errorf("expected cache miss after TTL expired")
	}
}
