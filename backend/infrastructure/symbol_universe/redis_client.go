package symbol_universe

import (
	"context"
	"fmt"
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
	return r.cli.Get(ctx, key).Result()
}

func (r *GoRedisClient) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.cli.Set(ctx, key, value, ttl).Err()
}

func (r *GoRedisClient) Del(ctx context.Context, keys ...string) error {
	return r.cli.Del(ctx, keys...).Err()
}

func (r *GoRedisClient) HSet(ctx context.Context, key string, fieldValues map[string]string) error {
	if len(fieldValues) == 0 {
		return nil
	}
	vals := make([]interface{}, 0, len(fieldValues)*2)
	for f, v := range fieldValues {
		vals = append(vals, f, v)
	}
	return r.cli.HSet(ctx, key, vals...).Err()
}

func (r *GoRedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	return r.cli.HGet(ctx, key, field).Result()
}

func (r *GoRedisClient) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.cli.Expire(ctx, key, ttl).Err()
}

func (r *GoRedisClient) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return r.cli.SetNX(ctx, key, value, ttl).Result()
}

func (r *GoRedisClient) MGet(ctx context.Context, keys ...string) ([]string, error) {
	vals, err := r.cli.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("mget %q: unexpected type %T", keys[i], v)
		}
		out[i] = s
	}
	return out, nil
}

func (r *GoRedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return r.cli.Eval(ctx, script, keys, args...).Result()
}
