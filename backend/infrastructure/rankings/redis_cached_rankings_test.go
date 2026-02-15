package rankings

import (
"context"
"encoding/json"
"errors"
"testing"
"time"

"pano_chart/backend/application/usecases"
"pano_chart/backend/domain"
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
result []usecases.RankedResult
err    error
called int
}

func (f *fakeRankingsUC) Execute(_ context.Context, _ usecases.GetRankingsRequest) ([]usecases.RankedResult, error) {
f.called++
return f.result, f.err
}

func sampleResults() []usecases.RankedResult {
return []usecases.RankedResult{
{
Symbol:     domain.NewSymbolUnsafe("BTCUSDT"),
TotalScore: 0.85,
Scores:     map[string]float64{"Gain/Loss": 0.9, "Sideways Consistency": 0.8},
Volume:     1000000,
},
{
Symbol:     domain.NewSymbolUnsafe("ETHUSDT"),
TotalScore: 0.70,
Scores:     map[string]float64{"Gain/Loss": 0.6, "Sideways Consistency": 0.75},
Volume:     500000,
},
}
}

func TestCacheMissCallsNext(t *testing.T) {
fr := &fakeRedis{store: map[string]string{}}
uc := &fakeRankingsUC{result: sampleResults()}
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1h"),
Sort:      usecases.SortByTotal,
}
results, err := cache.Execute(context.Background(), req)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
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
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1h"),
Sort:      usecases.SortByTotal,
}
_, _ = cache.Execute(context.Background(), req)

key := "rankings:1h:total"
if _, ok := fr.store[key]; !ok {
t.Errorf("expected value to be stored in redis at key %q", key)
}
}

func TestCacheHitDoesNotCallNext(t *testing.T) {
fr := &fakeRedis{store: map[string]string{}}
uc := &fakeRankingsUC{result: sampleResults()}
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

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
results, err := cache.Execute(context.Background(), req)
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
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

reqGain := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("4h"),
Sort:      usecases.SortByGain,
}
_, _ = cache.Execute(context.Background(), reqGain)

keyGain := "rankings:4h:gain"
if _, ok := fr.store[keyGain]; !ok {
t.Errorf("expected cache key %q, but not found", keyGain)
}

reqVol := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("4h"),
Sort:      usecases.SortByVolume,
}
_, _ = cache.Execute(context.Background(), reqVol)

keyVol := "rankings:4h:volume"
if _, ok := fr.store[keyVol]; !ok {
t.Errorf("expected cache key %q, but not found", keyVol)
}
}

func TestRedisGetFailureFallsThrough(t *testing.T) {
fr := &fakeRedis{store: map[string]string{}, fail: true}
uc := &fakeRankingsUC{result: sampleResults()}
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1h"),
Sort:      usecases.SortByTotal,
}
results, err := cache.Execute(context.Background(), req)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
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
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1h"),
Sort:      usecases.SortByTotal,
}
_, err := cache.Execute(context.Background(), req)
if err == nil {
t.Fatal("expected error from next, got nil")
}
if _, ok := fr.store["rankings:1h:total"]; ok {
t.Error("should not cache when next returns an error")
}
}

func TestEmptyResultsCached(t *testing.T) {
fr := &fakeRedis{store: map[string]string{}}
uc := &fakeRankingsUC{result: []usecases.RankedResult{}}
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1d"),
Sort:      usecases.SortByTotal,
}
results, err := cache.Execute(context.Background(), req)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if len(results) != 0 {
t.Errorf("expected 0 results, got %d", len(results))
}
key := "rankings:1d:total"
if _, ok := fr.store[key]; !ok {
t.Errorf("empty results should still be cached")
}
}

func TestScoresPreservedThroughCache(t *testing.T) {
fr := &fakeRedis{store: map[string]string{}}
uc := &fakeRankingsUC{result: sampleResults()}
cache := NewRedisCachedRankings(uc, fr, time.Minute, "rankings")

req := usecases.GetRankingsRequest{
Timeframe: domain.NewTimeframeUnsafe("1h"),
Sort:      usecases.SortByTotal,
}

// Populate cache
_, _ = cache.Execute(context.Background(), req)

// Read from cache
results, err := cache.Execute(context.Background(), req)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
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

key := "rankings:1h:total"
raw := fr.store[key]
var items []cachedRankedResult
if err := json.Unmarshal([]byte(raw), &items); err != nil {
t.Fatalf("failed to unmarshal cached JSON: %v", err)
}
if len(items) != 2 {
t.Fatalf("expected 2 cached items, got %d", len(items))
}
if items[0].Symbol != "BTCUSDT" {
t.Errorf("expected cached symbol BTCUSDT, got %s", items[0].Symbol)
}
}
