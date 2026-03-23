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
