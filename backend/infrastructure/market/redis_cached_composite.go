package market

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pano_chart/backend/application/market/metrics"
	mkt "pano_chart/backend/domain/market"
)

// CompositeRedisClient abstracts Redis operations for the cache.
type CompositeRedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedComposite is a decorator that caches CompositeIndexService
// results in Redis. Key format: {prefix}:{timeframe}:{limit}.
type RedisCachedComposite struct {
	next      *metrics.CompositeIndexService
	redis     CompositeRedisClient
	ttl       time.Duration
	keyPrefix string
}

// NewRedisCachedComposite constructs the cache decorator.
func NewRedisCachedComposite(
	next *metrics.CompositeIndexService,
	redis CompositeRedisClient,
	ttl time.Duration,
	keyPrefix string,
) *RedisCachedComposite {
	return &RedisCachedComposite{
		next:      next,
		redis:     redis,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

// Calculate tries the cache first, otherwise delegates and stores.
func (c *RedisCachedComposite) Calculate(ctx context.Context, timeframe string, limit int) (mkt.CompositeIndex, error) {
	key := fmt.Sprintf("%s:%s:%d", c.keyPrefix, timeframe, limit)

	// 1. Attempt cache hit
	cached, err := c.redis.Get(ctx, key)
	if err == nil && cached != "" {
		var idx mkt.CompositeIndex
		if unmarshalErr := json.Unmarshal([]byte(cached), &idx); unmarshalErr == nil {
			return idx, nil
		}
	}

	// 2. Cache miss — compute
	idx, err := c.next.Calculate(ctx, timeframe, limit)
	if err != nil {
		return mkt.CompositeIndex{}, err
	}

	// 3. Store (best-effort)
	data, marshalErr := json.Marshal(idx)
	if marshalErr == nil {
		_ = c.redis.Set(ctx, key, string(data), c.ttl)
	}

	return idx, nil
}
