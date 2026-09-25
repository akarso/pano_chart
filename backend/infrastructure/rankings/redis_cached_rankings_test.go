package rankings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	domainsignal "pano_chart/backend/domain/signal"
)

type fakeRedis struct {
	store map[string]string
	fail  bool
}

func (f *fakeRedis) Get(_ context.Context, key string) (string, error) {
	if f.fail {
		return "", errors.New("redis fail")
	}
	return f.store[key], nil
}

func (f *fakeRedis) Set(_ context.Context, key string, value string, _ time.Duration) error {
	if f.fail {
		return errors.New("redis fail")
	}
	f.store[key] = value
	return nil
}

type fakeRankingsUC struct {
	result usecases.RankingsResult
	err    error
	called int
}

func (f *fakeRankingsUC) Execute(_ context.Context, _ usecases.GetRankingsRequest) (usecases.RankingsResult, error) {
	f.called++
	return f.result, f.err
}

func f64(v float64) *float64 { return &v }

func sampleResults() usecases.RankingsResult {
	return usecases.RankingsResult{
		RSAvailable: true,
		Sort:        usecases.SortByTotal,
		Results: []usecases.RankedResult{
			{
				Symbol:           domain.NewSymbolUnsafe("BTCUSDT"),
				TotalScore:       0.85,
				Scores:           map[string]float64{"Gain/Loss": 0.9, "Sideways Consistency": 0.8},
				Volume:           1000000,
				RelativeStrength: f64(0.03),
				Beta:             f64(1.2),
				RSRank:           f64(1.0),
			},
			{
				Symbol:           domain.NewSymbolUnsafe("ETHUSDT"),
				TotalScore:       0.70,
				Scores:           map[string]float64{"Gain/Loss": 0.6, "Sideways Consistency": 0.75},
				Volume:           500000,
				RelativeStrength: f64(-0.01),
				Beta:             f64(0.8),
				RSRank:           f64(0.0),
			},
		},
	}
}

func TestCacheMissCallsNext(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	out, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if uc.called != 1 {
		t.Errorf("expected next to be called once, got %d", uc.called)
	}
}

func TestStoresInRedisAfterMiss(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	_, _ = cache.Execute(context.Background(), req)

	key := "rankings_v2:1h:total:default"
	if _, ok := fr.store[key]; !ok {
		t.Errorf("expected value to be stored in redis at key %q", key)
	}
}

func TestCacheHitDoesNotCallNext(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}

	// First call - cache miss
	_, _ = cache.Execute(context.Background(), req)
	if uc.called != 1 {
		t.Fatalf("expected 1 call after first execute, got %d", uc.called)
	}

	// Second call - cache hit
	out, err := cache.Execute(context.Background(), req)
	results := out.Results
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uc.called != 1 {
		t.Errorf("expected next NOT called on cache hit, called %d times", uc.called)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results from cache, got %d", len(results))
	}
}

func TestCacheKeyIncludesSortMode(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	reqGain := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("4h"),
		Sort:      usecases.SortByGain,
	}
	_, _ = cache.Execute(context.Background(), reqGain)

	keyGain := "rankings_v2:4h:gain:default"
	if _, ok := fr.store[keyGain]; !ok {
		t.Errorf("expected cache key %q, but not found", keyGain)
	}

	reqVol := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("4h"),
		Sort:      usecases.SortByVolume,
	}
	_, _ = cache.Execute(context.Background(), reqVol)

	keyVol := "rankings_v2:4h:volume:default"
	if _, ok := fr.store[keyVol]; !ok {
		t.Errorf("expected cache key %q, but not found", keyVol)
	}
}

func TestCacheKeyIncludesSidewaysAlgo(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	reqV2 := usecases.GetRankingsRequest{
		Timeframe:    domain.NewTimeframeUnsafe("1h"),
		Sort:         usecases.SortByTotal,
		SidewaysAlgo: usecases.SidewaysAlgoV2,
	}
	_, _ = cache.Execute(context.Background(), reqV2)

	keyV2 := "rankings_v2:1h:total:v2"
	if _, ok := fr.store[keyV2]; !ok {
		t.Errorf("expected cache key %q, but not found", keyV2)
	}

	// Default algo request should produce a different key
	reqDefault := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	_, _ = cache.Execute(context.Background(), reqDefault)

	keyDefault := "rankings_v2:1h:total:default"
	if _, ok := fr.store[keyDefault]; !ok {
		t.Errorf("expected cache key %q, but not found", keyDefault)
	}
}

func TestRedisGetFailureFallsThrough(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}, fail: true}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	out, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 2 {
		t.Errorf("expected 2 results on redis failure fallback, got %d", len(results))
	}
	if uc.called != 1 {
		t.Errorf("expected next called on redis failure, called %d", uc.called)
	}
}

func TestNextErrorPropagated(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{err: errors.New("next failed")}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	_, err := cache.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from next, got nil")
	}
	if _, ok := fr.store["rankings_v2:1h:total:default"]; ok {
		t.Error("should not cache when next returns an error")
	}
}

func TestEmptyResultsCached(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: usecases.RankingsResult{Results: []usecases.RankedResult{}, Sort: usecases.SortByTotal}}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1d"),
		Sort:      usecases.SortByTotal,
	}
	out, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	key := "rankings_v2:1d:total:default"
	if _, ok := fr.store[key]; !ok {
		t.Errorf("empty results should still be cached")
	}
}

func TestScoresPreservedThroughCache(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: sampleResults()}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}

	// Populate cache
	_, _ = cache.Execute(context.Background(), req)

	// Read from cache
	out, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results := out.Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	r0 := results[0]
	if r0.Symbol.String() != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", r0.Symbol.String())
	}
	if r0.TotalScore != 0.85 {
		t.Errorf("expected total score 0.85, got %f", r0.TotalScore)
	}
	if r0.Scores["Gain/Loss"] != 0.9 {
		t.Errorf("expected Gain/Loss 0.9, got %f", r0.Scores["Gain/Loss"])
	}
	if r0.Volume != 1000000 {
		t.Errorf("expected volume 1000000, got %f", r0.Volume)
	}

	key := "rankings_v2:1h:total:default"
	raw := fr.store[key]
	var payload cachedRankingsPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failed to unmarshal cached JSON: %v", err)
	}
	if !payload.RSAvailable {
		t.Fatal("cached payload should preserve rsAvailable")
	}
	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 cached items, got %d", len(payload.Results))
	}
	if payload.Results[0].RelativeStrength == nil || *payload.Results[0].RelativeStrength != 0.03 {
		t.Fatalf("cached rs=%v want 0.03", payload.Results[0].RelativeStrength)
	}
	if payload.Results[0].Beta == nil || *payload.Results[0].Beta != 1.2 {
		t.Fatalf("cached beta=%v want 1.2", payload.Results[0].Beta)
	}
	if payload.Results[0].RSRank == nil || *payload.Results[0].RSRank != 1.0 {
		t.Fatalf("cached rsRank=%v want 1", payload.Results[0].RSRank)
	}
	if r0.RelativeStrength == nil || *r0.RelativeStrength != 0.03 {
		t.Fatalf("round-trip rs=%v", r0.RelativeStrength)
	}
}

type capturingBadgeEmitter struct {
	n int
}

func (c *capturingBadgeEmitter) Emit(_ context.Context, _ domainsignal.Signal) bool {
	c.n++
	return true
}

func TestCacheHitEmitsBadgeSignals(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	badged := []usecases.RankedResult{
		{
			Symbol:            domain.NewSymbolUnsafe("BTCUSDT"),
			TotalScore:        0.9,
			MaxPercentile:     1,
			BadgeComponent:    "trend",
			Sparkline:         []float64{100, 110},
			SignalPrice:       110,
			SignalATR:         2,
			DominantComponent: "trend",
		},
	}
	uc := &fakeRankingsUC{result: usecases.RankingsResult{
		Results:     badged,
		RSAvailable: true,
		Sort:        usecases.SortByTotal,
	}}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")
	cap := &capturingBadgeEmitter{}
	cache.SetSignalEmitter(cap)

	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1m"),
		Sort:      usecases.SortByTotal,
	}
	// Miss: underlying UC would normally emit; fake does not — decorator only
	// emits on hit. Warm the cache, then hit.
	_, _ = cache.Execute(context.Background(), req)
	if cap.n != 0 {
		t.Fatalf("miss path should not double-emit from decorator, got %d", cap.n)
	}
	_, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if cap.n != 1 {
		t.Fatalf("cache hit must emit badge signals, got %d", cap.n)
	}
}

func TestLeadersCacheKey(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	rs, beta, rank := 0.05, 1.5, 1.0
	uc := &fakeRankingsUC{result: usecases.RankingsResult{
		RSAvailable:   true,
		Sort:          usecases.SortByLeaders,
		RequestedSort: usecases.SortByLeaders,
		Results: []usecases.RankedResult{{
			Symbol:           domain.NewSymbolUnsafe("HOTUSDT"),
			TotalScore:       0.5,
			RelativeStrength: &rs,
			Beta:             &beta,
			RSRank:           &rank,
		}},
	}}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")
	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByLeaders,
	}
	_, _ = cache.Execute(context.Background(), req)
	key := "rankings_v2:1h:leaders:default"
	raw, ok := fr.store[key]
	if !ok {
		t.Fatalf("expected leaders key, store=%v", fr.store)
	}
	var payload cachedRankingsPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.RSAvailable || payload.Sort != "leaders" {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Results[0].RelativeStrength == nil || *payload.Results[0].RelativeStrength != 0.05 {
		t.Fatalf("rs=%v", payload.Results[0].RelativeStrength)
	}
	// Hit path round-trip
	out, err := cache.Execute(context.Background(), req)
	if err != nil || uc.called != 1 {
		t.Fatalf("err=%v called=%d", err, uc.called)
	}
	if !out.RSAvailable || out.Results[0].RelativeStrength == nil || *out.Results[0].RelativeStrength != 0.05 {
		t.Fatalf("hit out=%+v", out)
	}
}

func TestLeadersTapeMissDoesNotPoisonCache(t *testing.T) {
	for _, mode := range []usecases.SortMode{usecases.SortByLeaders, usecases.SortByLaggards} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			fr := &fakeRedis{store: map[string]string{}}
			failing := &fakeRankingsUC{result: usecases.RankingsResult{
				RSAvailable:   false,
				Sort:          usecases.SortByTotal,
				RequestedSort: mode,
				Results: []usecases.RankedResult{{
					Symbol:     domain.NewSymbolUnsafe("BTCUSDT"),
					TotalScore: 0.9,
				}},
			}}
			cache := NewRedisCachedRankings(failing, fr, time.Minute, "rankings_v2")
			req := usecases.GetRankingsRequest{
				Timeframe: domain.NewTimeframeUnsafe("1h"),
				Sort:      mode,
			}
			out, err := cache.Execute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if out.RSAvailable || out.Sort != usecases.SortByTotal {
				t.Fatalf("fallback out=%+v", out)
			}
			key := "rankings_v2:1h:" + string(mode) + ":default"
			if _, ok := fr.store[key]; ok {
				t.Fatalf("must not SET %s on RSUnavailable fallback", key)
			}

			rs, beta, rank := 0.02, 1.1, 1.0
			live := &fakeRankingsUC{result: usecases.RankingsResult{
				RSAvailable:   true,
				Sort:          mode,
				RequestedSort: mode,
				Results: []usecases.RankedResult{{
					Symbol:           domain.NewSymbolUnsafe("BTCUSDT"),
					RelativeStrength: &rs,
					Beta:             &beta,
					RSRank:           &rank,
				}},
			}}
			cache2 := NewRedisCachedRankings(live, fr, time.Minute, "rankings_v2")
			out2, err := cache2.Execute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if live.called != 1 {
				t.Fatalf("live next called=%d want 1", live.called)
			}
			if !out2.RSAvailable || out2.Sort != mode {
				t.Fatalf("recovered out=%+v", out2)
			}
		})
	}
}

func TestTotalTapeMissDoesNotPoisonCache(t *testing.T) {
	for _, mode := range []usecases.SortMode{
		usecases.SortByTotal, usecases.SortByGain, usecases.SortByTrend,
	} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			fr := &fakeRedis{store: map[string]string{}}
			failing := &fakeRankingsUC{result: usecases.RankingsResult{
				RSAvailable:   false,
				Sort:          mode,
				RequestedSort: mode,
				Results: []usecases.RankedResult{{
					Symbol:     domain.NewSymbolUnsafe("BTCUSDT"),
					TotalScore: 0.9,
				}},
			}}
			cache := NewRedisCachedRankings(failing, fr, time.Minute, "rankings_v2")
			req := usecases.GetRankingsRequest{
				Timeframe: domain.NewTimeframeUnsafe("1h"),
				Sort:      mode,
			}
			out, err := cache.Execute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if out.RSAvailable {
				t.Fatalf("expected rsAvailable=false, got %+v", out)
			}
			key := "rankings_v2:1h:" + string(mode) + ":default"
			if _, ok := fr.store[key]; ok {
				t.Fatalf("must not SET %s when tape miss leaves RS unavailable", key)
			}

			rs, beta, rank := 0.04, 1.0, 1.0
			live := &fakeRankingsUC{result: usecases.RankingsResult{
				RSAvailable:   true,
				Sort:          mode,
				RequestedSort: mode,
				Results: []usecases.RankedResult{{
					Symbol:           domain.NewSymbolUnsafe("BTCUSDT"),
					TotalScore:       0.9,
					RelativeStrength: &rs,
					Beta:             &beta,
					RSRank:           &rank,
				}},
			}}
			cache2 := NewRedisCachedRankings(live, fr, time.Minute, "rankings_v2")
			out2, err := cache2.Execute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if live.called != 1 {
				t.Fatalf("live next called=%d want 1", live.called)
			}
			if !out2.RSAvailable || out2.Results[0].RelativeStrength == nil {
				t.Fatalf("recovered out=%+v", out2)
			}
		})
	}
}

func TestRSDisabledStillCaches(t *testing.T) {
	fr := &fakeRedis{store: map[string]string{}}
	uc := &fakeRankingsUC{result: usecases.RankingsResult{
		RSAvailable: false,
		RSDisabled:  true,
		Sort:        usecases.SortByTotal,
		Results: []usecases.RankedResult{{
			Symbol:     domain.NewSymbolUnsafe("BTCUSDT"),
			TotalScore: 0.5,
		}},
	}}
	cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings_v2")
	req := usecases.GetRankingsRequest{
		Timeframe: domain.NewTimeframeUnsafe("1h"),
		Sort:      usecases.SortByTotal,
	}
	_, err := cache.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	key := "rankings_v2:1h:total:default"
	if _, ok := fr.store[key]; !ok {
		t.Fatalf("RSDisabled must still SET %s", key)
	}
}
