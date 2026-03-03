package feargreed_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/infrastructure/feargreed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeRedis struct {
	store    map[string]string
	getErr   error
	setErr   error
	setCalls int
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{store: map[string]string{}}
}

func (f *fakeRedis) Get(_ context.Context, key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	v, ok := f.store[key]
	if !ok {
		return "", errors.New("redis: nil")
	}
	return v, nil
}

func (f *fakeRedis) Set(_ context.Context, key string, value string, _ time.Duration) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	f.store[key] = value
	return nil
}

type fakeUseCase struct {
	result *usecases.FearGreedResult
	err    error
	calls  int
}

func (f *fakeUseCase) Execute(_ context.Context) (*usecases.FearGreedResult, error) {
	f.calls++
	return f.result, f.err
}

// ---- tests ----

func TestRedisCached_CacheMiss_FetchesAndStores(t *testing.T) {
	redis := newFakeRedis()
	uc := &fakeUseCase{result: &usecases.FearGreedResult{
		Value:               42,
		ValueClassification: "Fear",
		Timestamp:           1772496000,
	}}
	cached := feargreed.NewRedisCachedFearGreed(uc, redis, 6*time.Hour)

	result, err := cached.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 42, result.Value)
	assert.Equal(t, "Fear", result.ValueClassification)
	assert.Equal(t, 1, uc.calls)
	assert.Equal(t, 1, redis.setCalls)

	// Verify cached value
	raw, _ := redis.Get(context.Background(), "feargreed:latest")
	assert.NotEmpty(t, raw)
}

func TestRedisCached_CacheHit_ReturnsWithoutFetch(t *testing.T) {
	redis := newFakeRedis()
	val := usecases.FearGreedResult{Value: 50, ValueClassification: "Neutral", Timestamp: 123}
	b, _ := json.Marshal(val)
	redis.store["feargreed:latest"] = string(b)

	uc := &fakeUseCase{result: &usecases.FearGreedResult{Value: 99}}
	cached := feargreed.NewRedisCachedFearGreed(uc, redis, 6*time.Hour)

	result, err := cached.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 50, result.Value)
	assert.Equal(t, "Neutral", result.ValueClassification)
	assert.Equal(t, 0, uc.calls, "should not call upstream on cache hit")
}

func TestRedisCached_RedisGetFails_FallsThrough(t *testing.T) {
	redis := newFakeRedis()
	redis.getErr = errors.New("connection refused")
	uc := &fakeUseCase{result: &usecases.FearGreedResult{Value: 30, ValueClassification: "Fear", Timestamp: 100}}
	cached := feargreed.NewRedisCachedFearGreed(uc, redis, 6*time.Hour)

	result, err := cached.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 30, result.Value)
	assert.Equal(t, 1, uc.calls)
}

func TestRedisCached_RedisSetFails_StillReturnsResult(t *testing.T) {
	redis := newFakeRedis()
	redis.setErr = errors.New("write fail")
	uc := &fakeUseCase{result: &usecases.FearGreedResult{Value: 75, ValueClassification: "Greed", Timestamp: 200}}
	cached := feargreed.NewRedisCachedFearGreed(uc, redis, 6*time.Hour)

	result, err := cached.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 75, result.Value)
}

func TestRedisCached_UpstreamFails_ReturnsError(t *testing.T) {
	redis := newFakeRedis()
	uc := &fakeUseCase{err: errors.New("api down")}
	cached := feargreed.NewRedisCachedFearGreed(uc, redis, 6*time.Hour)

	_, err := cached.Execute(context.Background())

	assert.Error(t, err)
	assert.Equal(t, 0, redis.setCalls, "should not cache errors")
}
