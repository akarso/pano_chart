package market

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
)

type fakeCompositeRedis struct {
	mu       sync.RWMutex
	store    map[string]string
	lastTTL  time.Duration
	setCount int
	failGet  bool
	failSet  bool
}

func newFakeCompositeRedis() *fakeCompositeRedis {
	return &fakeCompositeRedis{store: map[string]string{}}
}

func (f *fakeCompositeRedis) Get(_ context.Context, key string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.failGet {
		return "", errors.New("redis get fail")
	}
	return f.store[key], nil
}

func (f *fakeCompositeRedis) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSet {
		return errors.New("redis set fail")
	}
	f.lastTTL = ttl
	f.setCount++
	f.store[key] = value
	return nil
}

type spyTapeCandleProvider struct {
	symbolsCalls int
	fetchCalls   int
	symbols      []domain.Symbol
	candles      map[string][]domain.Candle
}

func (s *spyTapeCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	s.symbolsCalls++
	return s.symbols, nil
}

func (s *spyTapeCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	s.fetchCalls++
	key := sym.String() + ":" + tf.String()
	cs, ok := s.candles[key]
	if !ok {
		return domain.CandleSeries{}, errors.New("missing")
	}
	if n < len(cs) {
		cs = cs[len(cs)-n:]
	}
	return domain.NewCandleSeries(sym, tf, cs)
}

func TestRedisCachedComposite_CalculateTape_RoundTrip(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.Timeframe1h
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 3)
	for i := 0; i < 3; i++ {
		c := 100.0 + float64(i)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*time.Hour),
			c, c+1, c-1, c, 1000,
		)
	}
	spy := &spyTapeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:1h": candles},
	}
	svc := metrics.NewCompositeIndexService(spy, 4)
	redis := newFakeCompositeRedis()
	cached := NewRedisCachedComposite(svc, redis, 3*time.Minute, "market_composite_v3")

	first, err := cached.CalculateTape(context.Background(), "1h", 110)
	if err != nil {
		t.Fatalf("first CalculateTape: %v", err)
	}
	pref := first.PreferredSeries()
	if pref.Len() < 2 {
		t.Fatalf("expected preferred series len >= 2, got %d", pref.Len())
	}
	firstClose, _ := pref.First()
	lastClose, _ := pref.Last()
	fetchesAfterMiss := spy.fetchCalls

	second, err := cached.CalculateTape(context.Background(), "1h", 110)
	if err != nil {
		t.Fatalf("second CalculateTape: %v", err)
	}
	if spy.fetchCalls != fetchesAfterMiss {
		t.Errorf("second call within TTL should perform zero candle fetches; fetches %d → %d",
			fetchesAfterMiss, spy.fetchCalls)
	}
	pref2 := second.PreferredSeries()
	if pref2.Len() != pref.Len() {
		t.Errorf("PreferredSeries().Len() mismatch: %d vs %d", pref.Len(), pref2.Len())
	}
	first2, _ := pref2.First()
	last2, _ := pref2.Last()
	if first2.Close() != firstClose.Close() || last2.Close() != lastClose.Close() {
		t.Errorf("first/last close mismatch: (%.4f,%.4f) vs (%.4f,%.4f)",
			firstClose.Close(), lastClose.Close(), first2.Close(), last2.Close())
	}

	key := "market_composite_v3:tape:1h:110"
	if _, ok := redis.store[key]; !ok {
		t.Errorf("expected cache key %q", key)
	}
	// 1h/2 = 30m > 3m base → TTL stays 3m
	if redis.lastTTL != 3*time.Minute {
		t.Errorf("expected TTL 3m for 1h, got %v", redis.lastTTL)
	}
}

func TestRedisCachedComposite_CalculateTape_TimeframeAwareTTL(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.Timeframe1m
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 3)
	for i := 0; i < 3; i++ {
		c := 100.0 + float64(i)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*time.Minute),
			c, c+1, c-1, c, 1000,
		)
	}
	spy := &spyTapeCandleProvider{
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:1m": candles},
	}
	svc := metrics.NewCompositeIndexService(spy, 4)
	redis := newFakeCompositeRedis()
	cached := NewRedisCachedComposite(svc, redis, 3*time.Minute, "market_composite_v3")

	if _, err := cached.CalculateTape(context.Background(), "1m", 50); err != nil {
		t.Fatalf("CalculateTape: %v", err)
	}
	// min(3m, 1m/2=30s) = 30s
	if redis.lastTTL != 30*time.Second {
		t.Errorf("expected TTL 30s for 1m, got %v", redis.lastTTL)
	}
}

func TestRedisCachedComposite_CalculateTape_CacheMissFallsThrough(t *testing.T) {
	// Empty universe returns success with an unusable tape; ensure the miss
	// path still works when Redis Get fails.
	svc := metrics.NewCompositeIndexService(&spyTapeCandleProvider{symbols: nil}, 4)
	redis := newFakeCompositeRedis()
	redis.failGet = true
	cached := NewRedisCachedComposite(svc, redis, time.Minute, "pfx")

	tape, err := cached.CalculateTape(context.Background(), "4h", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tape.Index.SymbolCount != 0 {
		t.Errorf("expected empty universe SymbolCount 0, got %d", tape.Index.SymbolCount)
	}
	if redis.setCount != 0 {
		t.Errorf("empty tape must not be cached; Set called %d times", redis.setCount)
	}
}

func TestRedisCachedComposite_CalculateTape_EmptyTapeNotCached_RecoversOnNextCall(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.Timeframe1h
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 3)
	for i := 0; i < 3; i++ {
		c := 100.0 + float64(i)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*time.Hour),
			c, c+1, c-1, c, 1000,
		)
	}
	spy := &recoveringTapeCandleProvider{
		failing: true,
		symbols: []domain.Symbol{sym},
		candles: map[string][]domain.Candle{"BTCUSDT:1h": candles},
	}
	svc := metrics.NewCompositeIndexService(spy, 4)
	redis := newFakeCompositeRedis()
	cached := NewRedisCachedComposite(svc, redis, 3*time.Minute, "empty")

	first, err := cached.CalculateTape(context.Background(), "1h", 110)
	if err != nil {
		t.Fatalf("first CalculateTape: %v", err)
	}
	if first.PreferredSeries().Len() >= 2 {
		t.Fatalf("expected unusable tape while provider failing, got len=%d", first.PreferredSeries().Len())
	}
	if redis.setCount != 0 {
		t.Fatalf("unusable tape must not be written to Redis; setCount=%d", redis.setCount)
	}

	spy.failing = false
	second, err := cached.CalculateTape(context.Background(), "1h", 110)
	if err != nil {
		t.Fatalf("second CalculateTape: %v", err)
	}
	if second.PreferredSeries().Len() < 2 {
		t.Fatalf("expected recovery to re-fetch usable tape, got len=%d", second.PreferredSeries().Len())
	}
	if redis.setCount != 1 {
		t.Errorf("usable tape should be cached once; setCount=%d", redis.setCount)
	}
	if spy.fetchCalls < 1 {
		t.Errorf("expected candle fetch after recovery")
	}
}

// recoveringTapeCandleProvider returns an empty universe while failing is
// true, then real candles once recovered.
type recoveringTapeCandleProvider struct {
	failing    bool
	fetchCalls int
	symbols    []domain.Symbol
	candles    map[string][]domain.Candle
}

func (r *recoveringTapeCandleProvider) Symbols(_ context.Context) ([]domain.Symbol, error) {
	if r.failing {
		return nil, nil
	}
	return r.symbols, nil
}

func (r *recoveringTapeCandleProvider) GetLastNCandles(_ context.Context, sym domain.Symbol, tf domain.Timeframe, n int) (domain.CandleSeries, error) {
	r.fetchCalls++
	key := sym.String() + ":" + tf.String()
	cs, ok := r.candles[key]
	if !ok {
		return domain.CandleSeries{}, errors.New("missing")
	}
	if n < len(cs) {
		cs = cs[len(cs)-n:]
	}
	return domain.NewCandleSeries(sym, tf, cs)
}

func TestRedisCachedComposite_CalculateTape_SingleflightCoalescesMisses(t *testing.T) {
	sym := domain.NewSymbolUnsafe("BTCUSDT")
	tf := domain.Timeframe1h
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]domain.Candle, 3)
	for i := 0; i < 3; i++ {
		c := 100.0 + float64(i)
		candles[i] = domain.NewCandleUnsafe(
			sym, tf, base.Add(time.Duration(i)*time.Hour),
			c, c+1, c-1, c, 1000,
		)
	}
	spy := &blockingTapeCandleProvider{
		spyTapeCandleProvider: spyTapeCandleProvider{
			symbols: []domain.Symbol{sym},
			candles: map[string][]domain.Candle{"BTCUSDT:1h": candles},
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := metrics.NewCompositeIndexService(spy, 4)
	redis := newFakeCompositeRedis()
	cached := NewRedisCachedComposite(svc, redis, 3*time.Minute, "sf")

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := cached.CalculateTape(context.Background(), "1h", 110)
			errs <- err
		}()
	}

	<-spy.started
	close(spy.release)

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("CalculateTape: %v", err)
		}
	}
	if spy.fetchCalls != 1 {
		t.Errorf("expected singleflight to coalesce to 1 candle fetch, got %d", spy.fetchCalls)
	}
}

// blockingTapeCandleProvider holds the first Symbols call until release is
// closed so concurrent CalculateTape callers pile up on the same miss.
type blockingTapeCandleProvider struct {
	spyTapeCandleProvider
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingTapeCandleProvider) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.spyTapeCandleProvider.Symbols(ctx)
}
