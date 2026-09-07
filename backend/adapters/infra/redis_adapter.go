package infra

import (
	"context"
	"time"
	"pano_chart/backend/infrastructure/symbol_universe"
)

// RedisMinimalAdapter adapts GoRedisClient to MinimalRedisClient interface.
type RedisMinimalAdapter struct {
	Inner *symbol_universe.GoRedisClient
}

func NewRedisMinimalAdapter(inner *symbol_universe.GoRedisClient) *RedisMinimalAdapter {
	return &RedisMinimalAdapter{Inner: inner}
}

func (r *RedisMinimalAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.Inner.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (r *RedisMinimalAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.Inner.Set(ctx, key, string(value), ttl)
}
