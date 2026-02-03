package composition_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/akarso/pano_chart/backend/cmd/server"
	"github.com/akarso/pano_chart/backend/domain"
)

// fakeRepo implements ports.CandleRepositoryPort for tests.
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

// fakeRedis for composition tests
type fakeRedis struct {
	store map[string][]byte
	getErr error
	setErr error
	lastKey string
	lastTTL time.Duration
}

func (f *fakeRedis) Get(key string) ([]byte, error) {
	if f.getErr != nil { return nil, f.getErr }
	b, ok := f.store[key]
	if !ok { return nil, http.ErrNoLocation }
	return b, nil
}
func (f *fakeRedis) Set(key string, value []byte, ttl time.Duration) error {
	if f.setErr != nil { return f.setErr }
	if f.store == nil { f.store = make(map[string][]byte) }
	f.store[key] = value
	f.lastKey = key
	f.lastTTL = ttl
	return nil
}

func TestComposition_WiresWithoutRedis(t *testing.T) {
	// Provide a fake repo and verify handler calls it
	series, _ := domain.NewCandleSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), []domain.Candle{})
	fake := &fakeRepo{series: series}

	h, err := server.NewApp(server.Config{Repo: fake})
	if err != nil {
		t.Fatalf("failed to wire app: %v", err)
	}

	// Handler should be reachable via httptest
	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTC&timeframe=1m&from=2026-01-01T12:00:00Z&to=2026-01-01T12:01:00Z", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !fake.called {
		t.Fatal("expected use case / repo to be called")
	}
}

func TestComposition_WiresWithRedis(t *testing.T) {
	// Provide a fake repo and fake redis and ensure wiring succeeds and caching is used
	// create a sample series with one candle so caching stores a value
	sym := domain.NewSymbolUnsafe("BTC")
	tf := domain.NewTimeframeUnsafe("1m")
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c := domain.NewCandleUnsafe(sym, tf, ts, 100, 110, 90, 105, 1000)
	series, _ := domain.NewCandleSeries(sym, tf, []domain.Candle{c})
	wrapped := &fakeRepo{series: series}
	redis := &fakeRedis{}

	h, err := server.NewApp(server.Config{Repo: wrapped, RedisClient: redis, CacheTTL: 1 * time.Minute})
	if err != nil {
		t.Fatalf("failed to wire app with redis: %v", err)
	}

	// Call handler to trigger caching
	req := httptest.NewRequest("GET", "/api/v1/candles?symbol=BTC&timeframe=1m&from=2026-01-01T12:00:00Z&to=2026-01-01T12:01:00Z", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// wrapped repo should be called on first request
	if !wrapped.called {
		t.Fatal("expected wrapped repo to be called on first request")
	}
	// Redis should have received a Set
	if redis.lastKey == "" {
		t.Fatal("expected redis to be set on miss")
	}
}

func TestComposition_HTTPHandlerIsReachable(t *testing.T) {
	series, _ := domain.NewCandleSeries(domain.NewSymbolUnsafe("BTC"), domain.NewTimeframeUnsafe("1m"), []domain.Candle{})
	fake := &fakeRepo{series: series}
	h, err := server.NewApp(server.Config{Repo: fake})
	if err != nil {
		t.Fatalf("failed to wire app: %v", err)
	}

	ts := httptest.NewServer(h)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/v1/candles?symbol=BTC&timeframe=1m&from=2026-01-01T12:00:00Z&to=2026-01-01T12:01:00Z")
	if err != nil {
		t.Fatalf("failed to reach handler: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}
}
