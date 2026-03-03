package feargreed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pano_chart/backend/application/usecases"
)

// RedisClient abstracts the Get/Set operations needed for caching.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedFearGreed wraps a FearGreedUseCase with a Redis-backed cache.
// On cache miss or Redis failure the underlying use case is called and the
// result is stored with the configured TTL. Redis failures never break the
// endpoint.
type RedisCachedFearGreed struct {
	next  usecases.FearGreedUseCase
	redis RedisClient
	ttl   time.Duration
	key   string
}

// NewRedisCachedFearGreed constructs the decorator.
func NewRedisCachedFearGreed(next usecases.FearGreedUseCase, redis RedisClient, ttl time.Duration) *RedisCachedFearGreed {
	return &RedisCachedFearGreed{
		next:  next,
		redis: redis,
		ttl:   ttl,
		key:   "feargreed:latest",
	}
}

// Execute implements FearGreedUseCase.
func (c *RedisCachedFearGreed) Execute(ctx context.Context) (*usecases.FearGreedResult, error) {
	// 1. Try cache
	cached, err := c.redis.Get(ctx, c.key)
	if err == nil && cached != "" {
		var result usecases.FearGreedResult
		if json.Unmarshal([]byte(cached), &result) == nil {
			return &result, nil
		}
		fmt.Printf("[RedisCachedFearGreed] unmarshal error for key %s\n", c.key)
	} else if err != nil {
		fmt.Printf("[RedisCachedFearGreed] redis GET error: %v\n", err)
	}

	// 2. Cache miss — call upstream
	result, ucErr := c.next.Execute(ctx)
	if ucErr != nil {
		return nil, ucErr
	}

	// 3. Store in cache
	b, marshalErr := json.Marshal(result)
	if marshalErr == nil {
		if setErr := c.redis.Set(ctx, c.key, string(b), c.ttl); setErr != nil {
			fmt.Printf("[RedisCachedFearGreed] redis SET error: %v\n", setErr)
		}
	}

	return result, nil
}
