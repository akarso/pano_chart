package risk_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	infra "pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/risk"
	"pano_chart/backend/domain"
)

// --- Fakes ---

// fakeFuturesPort's three methods now run concurrently under
// BinanceFuturesDataProvider.Get (CR follow-up, PR-081), so its call
// counters and recorded symbols need mutex protection.
type fakeFuturesPort struct {
	funding   float64
	oi        []float64
	longRatio float64

	fundingErr   error
	oiErr        error
	longRatioErr error

	// blockUntilCancel makes FundingRate/OpenInterestHistory block on their
	// ctx instead of returning immediately — used to prove Get doesn't wait
	// for a slow call once another one has already failed. These two (not
	// LongShortRatio) are the ones that block so the discriminator works
	// regardless of implementation strategy: FundingRate is first in this
	// type's program order, so a sequential "return on first error"
	// implementation would already look fast if only the *last*-called
	// method failed — blocking the *earlier* ones is what actually
	// distinguishes "genuinely concurrent with ctx cancellation" from
	// "sequential, but the failing call happens to run first".
	blockUntilCancel bool

	mu             sync.Mutex
	fundingCalls   int
	oiCalls        int
	longRatioCalls int
	// lastXSymbol records the symbol each method last received, so tests
	// can assert on the normalized form Get is supposed to pass through.
	lastFundingSymbol   string
	lastOISymbol        string
	lastLongRatioSymbol string
}

func (f *fakeFuturesPort) FundingRate(ctx context.Context, symbol string) (float64, error) {
	f.mu.Lock()
	f.fundingCalls++
	f.lastFundingSymbol = symbol
	f.mu.Unlock()
	if f.blockUntilCancel {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	if f.fundingErr != nil {
		return 0, f.fundingErr
	}
	return f.funding, nil
}

func (f *fakeFuturesPort) OpenInterestHistory(ctx context.Context, symbol string) ([]float64, error) {
	f.mu.Lock()
	f.oiCalls++
	f.lastOISymbol = symbol
	f.mu.Unlock()
	if f.blockUntilCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.oiErr != nil {
		return nil, f.oiErr
	}
	return f.oi, nil
}

func (f *fakeFuturesPort) LongShortRatio(_ context.Context, symbol string) (float64, error) {
	f.mu.Lock()
	f.longRatioCalls++
	f.lastLongRatioSymbol = symbol
	f.mu.Unlock()
	if f.longRatioErr != nil {
		return 0, f.longRatioErr
	}
	return f.longRatio, nil
}

type fakeCandleRepo struct {
	series domain.CandleSeries
	err    error
}

func (f *fakeCandleRepo) GetSeries(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _, _ time.Time) (domain.CandleSeries, error) {
	return f.series, f.err
}

func (f *fakeCandleRepo) GetLastNCandles(_ context.Context, _ domain.Symbol, _ domain.Timeframe, _ int) (domain.CandleSeries, error) {
	if f.err != nil {
		return domain.CandleSeries{}, f.err
	}
	return f.series, nil
}

// makeCandleSeries builds a simple n-candle 4h series with a fixed close
// (100) and a high/low band, so Price and the nearest-cluster proxy are
// both predictable in tests.
func makeCandleSeries(t *testing.T, n int) domain.CandleSeries {
	t.Helper()
	sym, err := domain.NewSymbol("BTCUSDT")
	if err != nil {
		t.Fatalf("NewSymbol: %v", err)
	}
	tf, err := domain.NewTimeframe("4h")
	if err != nil {
		t.Fatalf("NewTimeframe: %v", err)
	}
	candles := make([]domain.Candle, n)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*4*time.Hour),
			100, 110, 90, 100, 1000,
		)
	}
	s, err := domain.NewCandleSeries(sym, tf, candles)
	if err != nil {
		t.Fatalf("NewCandleSeries: %v", err)
	}
	return s
}

// --- Tests ---

func TestBinanceFuturesDataProvider_ImplementsDataProvider(t *testing.T) {
	var _ risk.DataProvider = risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
}

func TestBinanceFuturesDataProvider_HappyPath_ComposesRealFuturesDataWithCandleDerivedPrice(t *testing.T) {
	series := makeCandleSeries(t, 50)
	futures := &fakeFuturesPort{
		funding:   0.0002,
		oi:        []float64{100, 110, 120, 130, 140, 150, 160, 170, 180, 200},
		longRatio: 0.62,
	}
	repo := &fakeCandleRepo{series: series}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	data, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Funding != 0.0002 {
		t.Errorf("expected Funding 0.0002 (from futures port), got %v", data.Funding)
	}
	if len(data.OISeries) != 10 {
		t.Fatalf("expected 10 OI entries, got %d", len(data.OISeries))
	}
	if data.LongRatio != 0.62 {
		t.Errorf("expected LongRatio 0.62 (from futures port), got %v", data.LongRatio)
	}
	if data.Price != 100 {
		t.Errorf("expected Price 100 (from last candle close), got %v", data.Price)
	}
	if data.NearestCluster == 0 {
		t.Errorf("expected a nonzero nearest-cluster proxy from candle data")
	}
}

func TestBinanceFuturesDataProvider_FundingRateError_FailsCall(t *testing.T) {
	// Regression test for PR-081 §5: a futures-data failure must fail the
	// whole call (SetupService already degrades Crowding to its zero
	// default on a non-cancellation FragilityProvider error) rather than
	// silently proceeding with partial/zero data.
	futures := &fakeFuturesPort{fundingErr: errors.New("no futures market for symbol")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "NOTASYMBOL", "4h")
	if err == nil {
		t.Fatal("expected error when FundingRate fails")
	}
}

// TestBinanceFuturesDataProvider_ConcurrentFetch_DoesNotWaitOutASlowCallAfterAnotherFails
// is the CR follow-up regression test for the sequential-latency concern:
// funding/OI/long-short/candles now run concurrently via errgroup instead
// of one after another, so a fast failure (FundingRate) should make Get
// return promptly instead of waiting for OpenInterestHistory/LongShortRatio
// to also finish. blockUntilCancel makes those two hang until their ctx is
// cancelled — proving errgroup.WithContext's cancellation actually
// propagates, not just that Get "returns an error eventually".
func TestBinanceFuturesDataProvider_ConcurrentFetch_DoesNotWaitOutASlowCallAfterAnotherFails(t *testing.T) {
	// LongShortRatio (the last of the three futures calls in this type's
	// own program order) fails immediately; FundingRate and
	// OpenInterestHistory (both earlier in program order) block until their
	// ctx is cancelled. A sequential "return on first error" implementation
	// would already look fast if the *first*-called method were the one
	// failing (nothing after it would ever run) — that would pass even
	// without real concurrency. Failing the *last* one instead means a
	// sequential implementation would hang on FundingRate first and never
	// even reach the failure, so this genuinely discriminates "concurrent
	// with ctx cancellation" from "sequential, but got lucky on ordering".
	futures := &fakeFuturesPort{
		blockUntilCancel: true, // FundingRate/OpenInterestHistory hang until Get's internal ctx is cancelled
		longRatioErr:     errors.New("no futures market for symbol"),
	}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := p.Get(context.Background(), "NOTASYMBOL", "4h")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error when LongShortRatio fails")
		}
		if !strings.Contains(err.Error(), "no futures market for symbol") {
			t.Errorf("expected the LongShortRatio error to surface, got: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("expected Get to return promptly once LongShortRatio failed, took %v", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get did not return within 5s — errgroup context cancellation likely isn't propagating to the blocked calls")
	}
}

func TestBinanceFuturesDataProvider_NormalizesSymbolCaseForFuturesCalls(t *testing.T) {
	// Regression test for the CR blocker: the HTTP handlers validate the
	// path segment via ParseSymbol but discard its normalized result, so a
	// lowercase request reaches this provider's Get as e.g. "btcusdt". The
	// candle repo path always normalized via domain.NewSymbol; the futures
	// calls used to get the raw string instead, reaching Binance
	// unnormalized and fragmenting RedisCachedFuturesData's per-symbol
	// cache keys by case variant.
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "btcusdt", "4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	futures.mu.Lock()
	defer futures.mu.Unlock()
	if futures.lastFundingSymbol != "BTCUSDT" {
		t.Errorf("expected FundingRate to receive normalized \"BTCUSDT\", got %q", futures.lastFundingSymbol)
	}
	if futures.lastOISymbol != "BTCUSDT" {
		t.Errorf("expected OpenInterestHistory to receive normalized \"BTCUSDT\", got %q", futures.lastOISymbol)
	}
	if futures.lastLongRatioSymbol != "BTCUSDT" {
		t.Errorf("expected LongShortRatio to receive normalized \"BTCUSDT\", got %q", futures.lastLongRatioSymbol)
	}
}

func TestBinanceFuturesDataProvider_OpenInterestHistoryError_FailsCall(t *testing.T) {
	// Note: since PR-081's CR follow-up made the three futures calls run
	// concurrently (errgroup), a successful LongShortRatio launched
	// alongside a failing OpenInterestHistory may still complete — that's
	// expected and fine; what matters is that Get's overall result is an
	// error, not that sibling calls never happened.
	futures := &fakeFuturesPort{funding: 0.0001, oiErr: errors.New("no data")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when OpenInterestHistory fails")
	}
}

func TestBinanceFuturesDataProvider_LongShortRatioError_FailsCall(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatioErr: errors.New("no data")}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 50)}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when LongShortRatio fails")
	}
}

func TestBinanceFuturesDataProvider_InvalidSymbol(t *testing.T) {
	p := risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
	_, err := p.Get(context.Background(), "", "4h")
	if err == nil {
		t.Fatal("expected error for invalid symbol")
	}
}

func TestBinanceFuturesDataProvider_InvalidTimeframe(t *testing.T) {
	p := risk.NewBinanceFuturesDataProvider(&fakeFuturesPort{}, &fakeCandleRepo{})
	_, err := p.Get(context.Background(), "BTCUSDT", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid timeframe")
	}
}

func TestBinanceFuturesDataProvider_CandleFetchError(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	repo := &fakeCandleRepo{err: errors.New("network failure")}
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error when candle fetch fails")
	}
}

func TestBinanceFuturesDataProvider_InsufficientCandleData(t *testing.T) {
	futures := &fakeFuturesPort{funding: 0.0001, oi: []float64{1, 2, 3}, longRatio: 0.5}
	repo := &fakeCandleRepo{series: makeCandleSeries(t, 1)} // fewer than 2 candles
	p := risk.NewBinanceFuturesDataProvider(futures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected error for insufficient candle data")
	}
}

// --- CR follow-up: cache poisoning by sibling cancellation ---

// blockingThenSucceedingFuturesPort's FundingRate blocks on its ctx (while
// fundingBlocks is true) instead of returning immediately — used to
// simulate a real in-flight Binance HTTP call that gets aborted by
// errgroup's shared-context cancellation when a sibling call fails, the
// same way the real BinanceFuturesClient's http.Client would behave.
type blockingThenSucceedingFuturesPort struct {
	mu            sync.Mutex
	fundingBlocks bool
	funding       float64
	oi            []float64
	longRatio     float64
}

func (f *blockingThenSucceedingFuturesPort) FundingRate(ctx context.Context, _ string) (float64, error) {
	f.mu.Lock()
	blocks := f.fundingBlocks
	f.mu.Unlock()
	if blocks {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return f.funding, nil
}

func (f *blockingThenSucceedingFuturesPort) OpenInterestHistory(_ context.Context, _ string) ([]float64, error) {
	return f.oi, nil
}

func (f *blockingThenSucceedingFuturesPort) LongShortRatio(_ context.Context, _ string) (float64, error) {
	return f.longRatio, nil
}

// fakeRiskCacheRedis is a minimal in-memory Get/Set fake satisfying
// infra.RedisCachedFuturesData's (unexported) redis-client interface
// structurally — the same pattern already used in
// tests/adapters/infra/redis_cached_futures_data_test.go.
type fakeRiskCacheRedis struct {
	mu    sync.Mutex
	store map[string]string
}

func (f *fakeRiskCacheRedis) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.store[key], nil
}

func (f *fakeRiskCacheRedis) Set(_ context.Context, key string, value string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value
	return nil
}

// TestBinanceFuturesDataProvider_CandleFailureDuringConcurrentFetch_DoesNotPoisonFuturesCache
// is the CR follow-up regression test for the cache-poisoning blocker: with
// the REAL RedisCachedFuturesData wired underneath (not the bare fake used
// by the other tests in this file), a candle-fetch failure cancels the
// shared errgroup context while FundingRate is still in flight. Before the
// fix, that in-flight call's context.Canceled error got cached as a
// "Binance says no" failure for failureTTL — poisoning every subsequent
// FundingRate lookup for this symbol, not just the one racing request.
func TestBinanceFuturesDataProvider_CandleFailureDuringConcurrentFetch_DoesNotPoisonFuturesCache(t *testing.T) {
	next := &blockingThenSucceedingFuturesPort{
		fundingBlocks: true, // blocks until Get's internal gctx is cancelled by the candle-fetch failure
		funding:       0.0002,
		oi:            []float64{1, 2, 3},
		longRatio:     0.5,
	}
	redis := &fakeRiskCacheRedis{store: map[string]string{}}
	cachedFutures := infra.NewRedisCachedFuturesData(next, redis, time.Minute, 45*time.Second)
	repo := &fakeCandleRepo{err: errors.New("candle backend hiccup — unrelated to Binance")}
	p := risk.NewBinanceFuturesDataProvider(cachedFutures, repo)

	_, err := p.Get(context.Background(), "BTCUSDT", "4h")
	if err == nil {
		t.Fatal("expected an error from the failing candle fetch")
	}

	// Call FundingRate again, directly through the cache decorator, now
	// that it can answer immediately. If the sibling cancellation poisoned
	// the cache, this returns a cached-failure error without calling next
	// again. If the fix holds, it's a genuine cache miss: next is called
	// and the real value comes back.
	next.mu.Lock()
	next.fundingBlocks = false
	next.mu.Unlock()

	rate, err2 := cachedFutures.FundingRate(context.Background(), "BTCUSDT")
	if err2 != nil {
		t.Fatalf("FundingRate cache appears poisoned by the sibling's cancellation: %v", err2)
	}
	if rate != 0.0002 {
		t.Errorf("expected the real funding rate 0.0002, got %v", rate)
	}
}
