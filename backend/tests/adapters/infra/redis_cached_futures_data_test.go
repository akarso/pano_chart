package infra_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	infra "pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/ports"
)

const (
	testTTL        = time.Minute
	testFailureTTL = 30 * time.Second
)

// --- Fakes ---

type fakeFuturesRedis struct {
	store map[string]string
	fail  bool
}

func (f *fakeFuturesRedis) Get(_ context.Context, key string) (string, error) {
	if f.fail {
		return "", errors.New("redis fail")
	}
	return f.store[key], nil
}

func (f *fakeFuturesRedis) Set(_ context.Context, key string, value string, _ time.Duration) error {
	if f.fail {
		return errors.New("redis fail")
	}
	f.store[key] = value
	return nil
}

type fakeFuturesDataPort struct {
	funding       float64
	oi            []float64
	longRatio     float64
	err           error
	fundingCalls  int
	oiCalls       int
	longShortCall int
}

func (f *fakeFuturesDataPort) FundingRate(_ context.Context, _ string) (float64, error) {
	f.fundingCalls++
	if f.err != nil {
		return 0, f.err
	}
	return f.funding, nil
}

func (f *fakeFuturesDataPort) OpenInterestHistory(_ context.Context, _ string) ([]float64, error) {
	f.oiCalls++
	if f.err != nil {
		return nil, f.err
	}
	return f.oi, nil
}

func (f *fakeFuturesDataPort) LongShortRatio(_ context.Context, _ string) (float64, error) {
	f.longShortCall++
	if f.err != nil {
		return 0, f.err
	}
	return f.longRatio, nil
}

func TestRedisCachedFuturesData_ImplementsPort(t *testing.T) {
	var _ ports.FuturesDataPort = infra.NewRedisCachedFuturesData(&fakeFuturesDataPort{}, &fakeFuturesRedis{store: map[string]string{}}, testTTL, testFailureTTL)
}

// --- FundingRate ---

func TestRedisCachedFuturesData_FundingRate_CacheHitSkipsNext(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{"futures:funding:BTCUSDT": "0.0001"}}
	next := &fakeFuturesDataPort{funding: 0.999}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	rate, err := cache.FundingRate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 0.0001 {
		t.Errorf("expected cached rate 0.0001, got %v", rate)
	}
	if next.fundingCalls != 0 {
		t.Errorf("next should not be called on cache hit")
	}
}

func TestRedisCachedFuturesData_FundingRate_CacheMissCallsNextAndStores(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{funding: 0.0002}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	rate, err := cache.FundingRate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 0.0002 {
		t.Errorf("expected rate 0.0002, got %v", rate)
	}
	if next.fundingCalls != 1 {
		t.Errorf("expected next to be called once, got %d", next.fundingCalls)
	}
	if fr.store["futures:funding:BTCUSDT"] == "" {
		t.Errorf("expected value to be cached")
	}
}

func TestRedisCachedFuturesData_FundingRate_ConfirmedUnavailableErrorIsFailureCached(t *testing.T) {
	// CR follow-up: only a *confirmed* symbol-data-unavailable error
	// (infra.ErrSymbolDataUnavailable — e.g. "Invalid symbol") is cached
	// (under failureTTL), so a known-bad symbol isn't re-fetched on every
	// single request. A transient/infrastructure error is NOT cached — see
	// TestRedisCachedFuturesData_FundingRate_TransientErrorNotCached below.
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{err: &infra.ErrSymbolDataUnavailable{Reason: "no futures market"}}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	_, err := cache.FundingRate(context.Background(), "NOTASYMBOL")
	if err == nil {
		t.Fatal("expected error from next")
	}
	if fr.store["futures:funding:NOTASYMBOL"] == "" {
		t.Errorf("expected the failure to be cached (under failureTTL)")
	}
}

func TestRedisCachedFuturesData_FundingRate_CachedFailureSkipsNext(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{err: &infra.ErrSymbolDataUnavailable{Reason: "no futures market"}}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	// First call: cache miss, fetch fails with a confirmed error, gets cached.
	if _, err := cache.FundingRate(context.Background(), "NOTASYMBOL"); err == nil {
		t.Fatal("expected error on first call")
	}
	if next.fundingCalls != 1 {
		t.Fatalf("expected next to be called once so far, got %d", next.fundingCalls)
	}

	// Second call: cached-failure hit — next must not be called again.
	_, err := cache.FundingRate(context.Background(), "NOTASYMBOL")
	if err == nil {
		t.Fatal("expected a cached-failure error on second call")
	}
	if !strings.Contains(err.Error(), "no futures market") {
		t.Errorf("expected the cached failure reason to surface, got: %v", err)
	}
	if next.fundingCalls != 1 {
		t.Errorf("expected next NOT to be called again on a cached-failure hit, got %d calls", next.fundingCalls)
	}
}

// TestRedisCachedFuturesData_FundingRate_TransientErrorNotCached is the CR
// follow-up regression test for the second cache-poisoning issue: a
// transient/infrastructure error (network blip, 429/5xx, malformed
// response — anything that isn't infra.ErrSymbolDataUnavailable) must NOT
// be cached as symbol-level unavailability, or every request for that
// symbol would keep reporting "unavailable" for failureTTL long after the
// transient issue actually cleared.
func TestRedisCachedFuturesData_FundingRate_TransientErrorNotCached(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{err: errors.New("connection reset by peer")} // plain error, not ErrSymbolDataUnavailable
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	// First call: cache miss, fetch fails transiently — must not be cached.
	if _, err := cache.FundingRate(context.Background(), "BTCUSDT"); err == nil {
		t.Fatal("expected error on first call")
	}
	if _, ok := fr.store["futures:funding:BTCUSDT"]; ok {
		t.Fatal("expected the transient failure NOT to be cached")
	}

	// Second call: still a genuine cache miss (nothing cached), so next is
	// called again — proving a fresh attempt is made instead of serving a
	// stale negative result. Simulate recovery: this time it succeeds.
	next.err = nil
	next.funding = 0.0003
	rate, err := cache.FundingRate(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("expected the retried call to succeed once the transient issue clears, got: %v", err)
	}
	if rate != 0.0003 {
		t.Errorf("expected the real funding rate 0.0003, got %v", rate)
	}
	if next.fundingCalls != 2 {
		t.Errorf("expected next to be called twice (no false cache hit in between), got %d", next.fundingCalls)
	}
}

// --- OpenInterestHistory ---

func TestRedisCachedFuturesData_OpenInterestHistory_CacheHitSkipsNext(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{"futures:oi:BTCUSDT": "[1,2,3]"}}
	next := &fakeFuturesDataPort{oi: []float64{9, 9, 9}}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	oi, err := cache.OpenInterestHistory(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []float64{1, 2, 3}
	if len(oi) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(oi))
	}
	for i := range want {
		if oi[i] != want[i] {
			t.Errorf("entry %d: expected %v, got %v", i, want[i], oi[i])
		}
	}
	if next.oiCalls != 0 {
		t.Errorf("next should not be called on cache hit")
	}
}

func TestRedisCachedFuturesData_OpenInterestHistory_CacheMissCallsNextAndStores(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{oi: []float64{10, 20, 30}}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	oi, err := cache.OpenInterestHistory(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(oi) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(oi))
	}
	if next.oiCalls != 1 {
		t.Errorf("expected next to be called once, got %d", next.oiCalls)
	}
	if fr.store["futures:oi:BTCUSDT"] == "" {
		t.Errorf("expected value to be cached")
	}
}

func TestRedisCachedFuturesData_OpenInterestHistory_CachedFailureSkipsNext(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}}
	next := &fakeFuturesDataPort{err: &infra.ErrSymbolDataUnavailable{Reason: "no futures market"}}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	if _, err := cache.OpenInterestHistory(context.Background(), "NOTASYMBOL"); err == nil {
		t.Fatal("expected error on first call")
	}
	if _, err := cache.OpenInterestHistory(context.Background(), "NOTASYMBOL"); err == nil {
		t.Fatal("expected a cached-failure error on second call")
	}
	if next.oiCalls != 1 {
		t.Errorf("expected next NOT to be called again on a cached-failure hit, got %d calls", next.oiCalls)
	}
}

// --- LongShortRatio ---

func TestRedisCachedFuturesData_LongShortRatio_CacheHitSkipsNext(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{"futures:longshort:BTCUSDT": "0.65"}}
	next := &fakeFuturesDataPort{longRatio: 0.01}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	ratio, err := cache.LongShortRatio(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ratio != 0.65 {
		t.Errorf("expected cached ratio 0.65, got %v", ratio)
	}
	if next.longShortCall != 0 {
		t.Errorf("next should not be called on cache hit")
	}
}

// --- Shared behavior ---

func TestRedisCachedFuturesData_FallsBackOnRedisFailure(t *testing.T) {
	fr := &fakeFuturesRedis{store: map[string]string{}, fail: true}
	next := &fakeFuturesDataPort{funding: 0.001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	cache := infra.NewRedisCachedFuturesData(next, fr, testTTL, testFailureTTL)

	if rate, err := cache.FundingRate(context.Background(), "BTCUSDT"); err != nil || rate != 0.001 {
		t.Errorf("FundingRate: expected 0.001, nil error; got %v, %v", rate, err)
	}
	if oi, err := cache.OpenInterestHistory(context.Background(), "BTCUSDT"); err != nil || len(oi) != 3 {
		t.Errorf("OpenInterestHistory: expected 3 entries, nil error; got %v, %v", oi, err)
	}
	if ratio, err := cache.LongShortRatio(context.Background(), "BTCUSDT"); err != nil || ratio != 0.5 {
		t.Errorf("LongShortRatio: expected 0.5, nil error; got %v, %v", ratio, err)
	}
}
