package infra_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"pano_chart/backend/adapters/infra"
	"pano_chart/backend/domain"
)

// fakeRedisClient is a test double for Redis operations.
type fakeRedisClient struct {
	store      map[string][]byte
	getErr     error
	setErr     error
	lastSetKey string
	lastSetTTL time.Duration
}

func newFakeRedis() *fakeRedisClient {
	return &fakeRedisClient{store: make(map[string][]byte)}
}

func (f *fakeRedisClient) Get(key string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	b, ok := f.store[key]
	if !ok {
		return nil, errors.New("cache miss")
	}
	return b, nil
}

func (f *fakeRedisClient) Set(key string, value []byte, ttl time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.store[key] = value
	f.lastSetKey = key
	f.lastSetTTL = ttl
	return nil
}

// fakeRepo is a stubbed wrapped repository implementation.
type fakeRepo struct {
	called bool
	series domain.CandleSeries
	err    error
}

func (f *fakeRepo) GetSeries(sym domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	f.called = true
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

func (f *fakeRepo) GetLastNCandles(sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

func buildSampleSeries() domain.CandleSeries {
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c := domain.NewCandleUnsafe(sym, tf, ts, 100, 110, 90, 105, 1000)
	series, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{c})
	return series
}

func TestRedisCandleRepository_ReturnsCachedResultOnHit(t *testing.T) {
	fake := newFakeRedis()
	series := buildSampleSeries()

	// serialize series into cache format
	all := series.All()
	items := make([]map[string]interface{}, 0, len(all))
	for _, c := range all {
		items = append(items, map[string]interface{}{
			"timestamp": c.Timestamp().Format(time.RFC3339),
			"open":      c.Open(),
			"high":      c.High(),
			"low":       c.Low(),
			"close":     c.Close(),
			"volume":    c.Volume(),
		})
	}
	b, _ := json.Marshal(items)
	key := "BTC|1m|2026-01-01T12:00:00Z|2026-01-01T12:01:00Z"
	fake.store[key] = b

	wrapped := &fakeRepo{}
	repo := infra.NewRedisCandleRepository(fake, wrapped, 5*time.Minute)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	res, err := repo.GetSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Len() != 1 {
		t.Fatalf("expected 1 candle, got %d", res.Len())
	}
	if wrapped.called {
		t.Fatal("did not expect wrapped repo to be called on cache hit")
	}
}

func TestRedisCandleRepository_DelegatesOnCacheMiss(t *testing.T) {
	fake := newFakeRedis()
	series := buildSampleSeries()
	wrapped := &fakeRepo{series: series}
	repo := infra.NewRedisCandleRepository(fake, wrapped, 5*time.Minute)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	res, err := repo.GetSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Len() != 1 {
		t.Fatalf("expected 1 candle, got %d", res.Len())
	}
	if !wrapped.called {
		t.Fatal("expected wrapped repo to be called on cache miss")
	}
}

func TestRedisCandleRepository_CachesResultAfterMiss(t *testing.T) {
	fake := newFakeRedis()
	series := buildSampleSeries()
	wrapped := &fakeRepo{series: series}
	ttl := 2 * time.Minute
	repo := infra.NewRedisCandleRepository(fake, wrapped, ttl)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ensure set was called
	expectedKey := "BTC|1m|2026-01-01T12:00:00Z|2026-01-01T12:01:00Z"
	if fake.lastSetKey != expectedKey {
		t.Fatalf("unexpected cache key: %s", fake.lastSetKey)
	}
	if fake.lastSetTTL != ttl {
		t.Fatalf("unexpected ttl: %v", fake.lastSetTTL)
	}
}

func TestRedisCandleRepository_IgnoresRedisErrors(t *testing.T) {
	fake := newFakeRedis()
	fake.getErr = errors.New("redis down")
	fake.setErr = errors.New("redis down")
	series := buildSampleSeries()
	wrapped := &fakeRepo{series: series}
	repo := infra.NewRedisCandleRepository(fake, wrapped, 1*time.Minute)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	res, err := repo.GetSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), from, to)
	if err != nil {
		t.Fatalf("expected redis errors to be ignored, got: %v", err)
	}
	if res.Len() != 1 {
		t.Fatalf("expected 1 candle, got %d", res.Len())
	}
}

func TestRedisCandleRepository_PropagatesRepositoryError(t *testing.T) {
	fake := newFakeRedis()
	wrapped := &fakeRepo{err: errors.New("backend fail")}
	repo := infra.NewRedisCandleRepository(fake, wrapped, 1*time.Minute)

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Minute)

	_, err := repo.GetSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), from, to)
	if err == nil {
		t.Fatal("expected repository error to propagate")
	}
}
