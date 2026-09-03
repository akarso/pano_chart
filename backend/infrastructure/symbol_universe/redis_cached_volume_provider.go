package symbol_universe

import (
	"context"
	"encoding/json"
	"time"
)

type VolumeProvider interface {
	Volumes(ctx context.Context) (map[string]float64, error)
}

type RedisCachedVolumeProvider struct {
	next  VolumeProvider
	redis RedisClient
	ttl   time.Duration
	key   string
}

func NewRedisCachedVolumeProvider(next VolumeProvider, redis RedisClient, ttl time.Duration, key string) *RedisCachedVolumeProvider {
	return &RedisCachedVolumeProvider{next: next, redis: redis, ttl: ttl, key: key}
}

func (r *RedisCachedVolumeProvider) Volumes(ctx context.Context) (map[string]float64, error) {
	cached, err := r.redis.Get(ctx, r.key)
	if err == nil && cached != "" {
		var m map[string]float64
		if err := json.Unmarshal([]byte(cached), &m); err == nil {
			return m, nil
		}
		// else: treat as cache miss
	}
	// Cache miss or error: call next
	m, err := r.next.Volumes(ctx)
	if err != nil {
		return nil, err
	}
	b, err2 := json.Marshal(m)
	if err2 == nil {
		_ = r.redis.Set(ctx, r.key, string(b), r.ttl) // ignore set error, fallback is fine
	}
	return m, nil
}
