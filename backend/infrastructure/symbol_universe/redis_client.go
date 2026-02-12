package symbol_universe

import (
	"context"
	"time"
	"github.com/redis/go-redis/v9"
)

type GoRedisClient struct {
	cli *redis.Client
}

func NewGoRedisClient(addr string) *GoRedisClient {
	cli := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &GoRedisClient{cli: cli}
}

func (r *GoRedisClient) Get(ctx context.Context, key string) (string, error) {
	val, err := r.cli.Get(ctx, key).Result()
	return val, err
}

func (r *GoRedisClient) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.cli.Set(ctx, key, value, ttl).Err()
}
