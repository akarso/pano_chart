package volatility_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	vol "pano_chart/backend/infrastructure/volatility"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregate_EmptyInput(t *testing.T) {
	result := vol.Aggregate(nil)
	assert.Empty(t, result.Buckets)
}

func TestAggregate_ProducesNonEmptyBuckets(t *testing.T) {
	candles := make([]vol.Candle, 100)
	base := int64(1_700_000_000_000)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100.5,
		}
	}
	result := vol.Aggregate(candles)
	assert.NotEmpty(t, result.Buckets)
	for _, b := range result.Buckets {
		assert.GreaterOrEqual(t, b.MinuteOfDay, 0)
		assert.Less(t, b.MinuteOfDay, 1440)
		assert.Greater(t, b.AvgMove, 0.0)
		assert.GreaterOrEqual(t, b.SpikeProb, 0.0)
		assert.LessOrEqual(t, b.SpikeProb, 1.0)
		assert.Greater(t, b.Normalized, 0.0)
	}
}

func TestAggregate_NormalizedAveragesCloseToOne(t *testing.T) {
	candles := make([]vol.Candle, 200)
	base := int64(1_700_000_000_000)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     50,
			High:     51,
			Low:      49,
			Close:    50.2,
		}
	}
	result := vol.Aggregate(candles)
	require.NotEmpty(t, result.Buckets)
	var sum float64
	for _, b := range result.Buckets {
		sum += b.Normalized
	}
	avg := sum / float64(len(result.Buckets))
	assert.InDelta(t, 1.0, avg, 0.01)
}

func TestAggregate_SpikeDetection(t *testing.T) {
	candles := make([]vol.Candle, 50)
	base := int64(1_700_000_000_000)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100,
		}
	}
	candles[49].Close = 120
	candles[49].High = 121
	result := vol.Aggregate(candles)
	require.NotEmpty(t, result.Buckets)
	var hasSpikeProb bool
	for _, b := range result.Buckets {
		if b.SpikeProb > 0 {
			hasSpikeProb = true
			break
		}
	}
	assert.True(t, hasSpikeProb)
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	return db
}

func TestCandleCache_StoreAndLoad(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cache, err := vol.NewCandleCacheFromDB(db)
	require.NoError(t, err)
	candles := []vol.Candle{
		{OpenTime: 1000, Open: 1, High: 2, Low: 0.5, Close: 1.5},
		{OpenTime: 2000, Open: 1.5, High: 2.5, Low: 1, Close: 2},
		{OpenTime: 3000, Open: 2, High: 3, Low: 1.5, Close: 2.5},
	}
	require.NoError(t, cache.Store("BTCUSDT", candles))
	loaded, err := cache.Load("BTCUSDT", 1000, 3000)
	require.NoError(t, err)
	assert.Len(t, loaded, 3)
	assert.Equal(t, int64(1000), loaded[0].OpenTime)
	assert.Equal(t, int64(3000), loaded[2].OpenTime)
}

func TestCandleCache_StoreDuplicatesIgnored(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cache, err := vol.NewCandleCacheFromDB(db)
	require.NoError(t, err)
	c := []vol.Candle{{OpenTime: 1000, Open: 1, High: 2, Low: 0.5, Close: 1.5}}
	require.NoError(t, cache.Store("BTCUSDT", c))
	require.NoError(t, cache.Store("BTCUSDT", c))
	n, err := cache.Count("BTCUSDT")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestCandleCache_MaxOpenTime(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cache, err := vol.NewCandleCacheFromDB(db)
	require.NoError(t, err)
	m, err := cache.MaxOpenTime("BTCUSDT")
	require.NoError(t, err)
	assert.Equal(t, int64(0), m)
	require.NoError(t, cache.Store("BTCUSDT", []vol.Candle{
		{OpenTime: 5000, Open: 1, High: 2, Low: 1, Close: 1.5},
		{OpenTime: 9000, Open: 1, High: 2, Low: 1, Close: 1.5},
	}))
	m, err = cache.MaxOpenTime("BTCUSDT")
	require.NoError(t, err)
	assert.Equal(t, int64(9000), m)
}

func TestCandleCache_LoadFiltersTimeRange(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cache, err := vol.NewCandleCacheFromDB(db)
	require.NoError(t, err)
	require.NoError(t, cache.Store("BTCUSDT", []vol.Candle{
		{OpenTime: 1000}, {OpenTime: 2000}, {OpenTime: 3000},
		{OpenTime: 4000}, {OpenTime: 5000},
	}))
	loaded, err := cache.Load("BTCUSDT", 2000, 4000)
	require.NoError(t, err)
	assert.Len(t, loaded, 3)
}

func TestCandleCache_SymbolIsolation(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cache, err := vol.NewCandleCacheFromDB(db)
	require.NoError(t, err)
	require.NoError(t, cache.Store("BTCUSDT", []vol.Candle{{OpenTime: 1000}}))
	require.NoError(t, cache.Store("ETHUSDT", []vol.Candle{{OpenTime: 2000}}))
	btc, err := cache.Load("BTCUSDT", 0, 9999)
	require.NoError(t, err)
	assert.Len(t, btc, 1)
	eth, err := cache.Load("ETHUSDT", 0, 9999)
	require.NoError(t, err)
	assert.Len(t, eth, 1)
}

func TestFetcher_ParsesKlines(t *testing.T) {
	payload := "[" +
		"[1700000000000,\"100\",\"101\",\"99\",\"100.5\",\"1000\",\"1700000059999\",\"100000\",\"50\",\"500\",\"50000\",\"0\"]," +
		"[1700000060000,\"100.5\",\"102\",\"100\",\"101\",\"2000\",\"1700000119999\",\"200000\",\"80\",\"800\",\"80000\",\"0\"]" +
		"]"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, payload)
	}))
	defer srv.Close()
	f := vol.NewFetcher(srv.Client())
	candles, err := f.FetchCandlesFromURL(context.Background(), srv.URL)
	require.NoError(t, err)
	require.Len(t, candles, 2)
	assert.Equal(t, int64(1700000000000), candles[0].OpenTime)
	assert.InDelta(t, 100.0, candles[0].Open, 0.001)
	assert.InDelta(t, 101.0, candles[0].High, 0.001)
	assert.InDelta(t, 99.0, candles[0].Low, 0.001)
	assert.InDelta(t, 100.5, candles[0].Close, 0.001)
}

func TestFetcher_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	f := vol.NewFetcher(srv.Client())
	_, err := f.FetchCandlesFromURL(context.Background(), srv.URL)
	assert.Error(t, err)
}

func TestSaveToFile_WritesValidJSON(t *testing.T) {
	result := vol.Result{
		Buckets: []vol.BucketResult{
			{MinuteOfDay: 0, AvgMove: 0.42, SpikeProb: 0.08, Normalized: 0.91},
			{MinuteOfDay: 60, AvgMove: 0.55, SpikeProb: 0.12, Normalized: 1.19},
		},
	}
	path := t.TempDir() + "/test_output.json"
	require.NoError(t, vol.SaveToFile(result, path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var loaded vol.Result
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Len(t, loaded.Buckets, 2)
	assert.InDelta(t, 0.42, loaded.Buckets[0].AvgMove, 0.001)
}

// ── DeriveTimeframe tests ──

func TestDeriveTimeframe_EmptyBase(t *testing.T) {
	r := vol.DeriveTimeframe(nil, 5, vol.TF5m)
	assert.Equal(t, vol.TF5m, r.Timeframe)
	assert.Empty(t, r.Buckets)
}

func TestDeriveTimeframe_GroupsCorrectly(t *testing.T) {
	base := make([]vol.BucketResult, 10)
	for i := range base {
		base[i] = vol.BucketResult{
			MinuteOfDay: i,
			AvgMove:     float64(i + 1),
			SpikeProb:   0.1,
			Normalized:  1.0,
		}
	}
	r := vol.DeriveTimeframe(base, 5, vol.TF5m)
	assert.Len(t, r.Buckets, 2)
	// First group: minutes 0-4, avg of 1,2,3,4,5 = 3.0
	assert.InDelta(t, 3.0, r.Buckets[0].AvgMove, 0.001)
	assert.Equal(t, 0, r.Buckets[0].MinuteOfDay)
	// Second group: minutes 5-9, avg of 6,7,8,9,10 = 8.0
	assert.InDelta(t, 8.0, r.Buckets[1].AvgMove, 0.001)
	assert.Equal(t, 5, r.Buckets[1].MinuteOfDay)
}

func TestDeriveTimeframe_PartialLastGroup(t *testing.T) {
	base := make([]vol.BucketResult, 7)
	for i := range base {
		base[i] = vol.BucketResult{
			MinuteOfDay: i,
			AvgMove:     1.0,
			SpikeProb:   0.0,
			Normalized:  1.0,
		}
	}
	r := vol.DeriveTimeframe(base, 5, vol.TF5m)
	// 5 + 2 → 2 groups
	assert.Len(t, r.Buckets, 2)
	// Partial group still averages correctly
	assert.InDelta(t, 1.0, r.Buckets[1].AvgMove, 0.001)
}

func TestBuildAllTimeframes_ProducesSixTimeframes(t *testing.T) {
	base := make([]vol.BucketResult, 1440)
	for i := range base {
		base[i] = vol.BucketResult{
			MinuteOfDay: i,
			AvgMove:     0.5,
			SpikeProb:   0.05,
			Normalized:  1.0,
		}
	}
	tfs := vol.BuildAllTimeframes(base)
	assert.Len(t, tfs, 6)
	assert.Equal(t, vol.TF1m, tfs[0].Timeframe)
	assert.Len(t, tfs[0].Buckets, 1440) // 1m: pass-through
	assert.Len(t, tfs[1].Buckets, 288)  // 5m
	assert.Len(t, tfs[2].Buckets, 96)   // 15m
	assert.Len(t, tfs[3].Buckets, 24)   // 1h
	assert.Len(t, tfs[4].Buckets, 6)    // 4h
	assert.Len(t, tfs[5].Buckets, 1)    // 1d
}

// ── Weekly tests ──

func TestBuildWeekly_EmptyInput(t *testing.T) {
	r := vol.BuildWeekly(nil, nil)
	assert.Empty(t, r.Buckets)
}

func TestBuildWeekly_ProducesBucketsPerDayOfWeek(t *testing.T) {
	// Generate 7 days of candles (1 per minute) starting on a known weekday.
	// 2023-11-13 is a Monday (Weekday()==1).
	base := int64(1699833600000) // 2023-11-13 00:00:00 UTC
	n := 7 * 1440
	candles := make([]vol.Candle, n)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100.5,
		}
	}
	atr := vol.ComputeATR(candles, 14)
	// Skip warmup period for both candles and atr.
	r := vol.BuildWeekly(candles[14:], atr[14:])
	assert.NotEmpty(t, r.Buckets)

	// Check that we have entries spanning multiple days of week.
	days := map[int]bool{}
	for _, b := range r.Buckets {
		days[b.MinuteOfWeek/1440] = true
		assert.Greater(t, b.AvgMove, 0.0)
		assert.GreaterOrEqual(t, b.SpikeProb, 0.0)
		assert.Greater(t, b.Normalized, 0.0)
	}
	assert.GreaterOrEqual(t, len(days), 6, "should cover at least 6 days of week")
}

func TestBuildWeekly_SortedByMinuteOfWeek(t *testing.T) {
	base := int64(1699833600000)
	n := 2 * 1440
	candles := make([]vol.Candle, n)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100.5,
		}
	}
	atr := vol.ComputeATR(candles, 14)
	r := vol.BuildWeekly(candles[14:], atr[14:])
	for i := 1; i < len(r.Buckets); i++ {
		assert.LessOrEqual(t, r.Buckets[i-1].MinuteOfWeek, r.Buckets[i].MinuteOfWeek)
	}
}

func TestBuildWeekly_NormalizedAveragesCloseToOne(t *testing.T) {
	base := int64(1699833600000)
	n := 3 * 1440
	candles := make([]vol.Candle, n)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: base + int64(i)*60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100.5,
		}
	}
	atr := vol.ComputeATR(candles, 14)
	r := vol.BuildWeekly(candles[14:], atr[14:])
	require.NotEmpty(t, r.Buckets)
	var sum float64
	for _, b := range r.Buckets {
		sum += b.Normalized
	}
	avg := sum / float64(len(r.Buckets))
	assert.InDelta(t, 1.0, avg, 0.01)
}

// ── SaveFullResult test ──

func TestSaveFullResult_WritesValidJSON(t *testing.T) {
	full := vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{Timeframe: vol.TF1m, Buckets: []vol.BucketResult{
				{MinuteOfDay: 0, AvgMove: 0.5, SpikeProb: 0.1, Normalized: 1.0},
			}},
		},
		Weekly: vol.WeeklyResult{
			Buckets: []vol.WeeklyBucket{
				{MinuteOfWeek: 0, AvgMove: 0.6, SpikeProb: 0.05, Normalized: 0.9},
			},
		},
	}
	path := t.TempDir() + "/full_output.json"
	require.NoError(t, vol.SaveFullResult(full, path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var loaded vol.FullResult
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Len(t, loaded.Intraday, 1)
	assert.Equal(t, vol.TF1m, loaded.Intraday[0].Timeframe)
	assert.Len(t, loaded.Weekly.Buckets, 1)
	assert.InDelta(t, 0.6, loaded.Weekly.Buckets[0].AvgMove, 0.001)
}

// ── ComputeATR export test ──

func TestComputeATR_Exported(t *testing.T) {
	candles := make([]vol.Candle, 20)
	for i := range candles {
		candles[i] = vol.Candle{
			OpenTime: int64(i) * 60_000,
			Open:     100,
			High:     101,
			Low:      99,
			Close:    100,
		}
	}
	atr := vol.ComputeATR(candles, 14)
	assert.Len(t, atr, 20)
	assert.Equal(t, 0.0, atr[0])
	assert.Greater(t, atr[19], 0.0)
}
