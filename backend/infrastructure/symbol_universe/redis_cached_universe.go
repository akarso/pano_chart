package symbol_universe

import (
	"context"
	"encoding/json"
	"time"
	"fmt"
	"pano_chart/backend/domain"
)

type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

type SymbolUniverseProvider interface {
	Symbols(ctx context.Context) ([]domain.Symbol, error)
}

type RedisCachedSymbolUniverse struct {
	next  SymbolUniverseProvider
	redis RedisClient
	ttl   time.Duration
	key   string
}

func NewRedisCachedSymbolUniverse(next SymbolUniverseProvider, redis RedisClient, ttl time.Duration, key string) *RedisCachedSymbolUniverse {
	return &RedisCachedSymbolUniverse{next: next, redis: redis, ttl: ttl, key: key}
}

func (r *RedisCachedSymbolUniverse) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	cached, err := r.redis.Get(ctx, r.key)
	if err == nil && cached != "" {
		var syms []string
		if err := json.Unmarshal([]byte(cached), &syms); err == nil {
			out := make([]domain.Symbol, 0, len(syms))
			for _, s := range syms {
				dsym, err := domain.NewSymbol(s)
				if err == nil {
					out = append(out, dsym)
				}
			}
			fmt.Printf("[RedisCachedSymbolUniverse] cache hit: %s, count=%d\n", r.key, len(out))
			return out, nil
		}
		fmt.Printf("[RedisCachedSymbolUniverse] cache unmarshal error, treating as miss: %v\n", err)
		// else: treat as cache miss
	} else {
		if err != nil {
			fmt.Printf("[RedisCachedSymbolUniverse] cache get error: %v\n", err)
		} else {
			fmt.Printf("[RedisCachedSymbolUniverse] cache miss: %s\n", r.key)
		}
	}
	// Cache miss or error: call next
	fmt.Printf("[RedisCachedSymbolUniverse] calling next provider for key: %s\n", r.key)
	syms, err := r.next.Symbols(ctx)
	if err != nil {
		fmt.Printf("[RedisCachedSymbolUniverse] error from next provider: %v\n", err)
		return nil, err
	}
	fmt.Printf("[RedisCachedSymbolUniverse] next provider returned count=%d\n", len(syms))
	// Serialize and store in Redis
	strs := make([]string, 0, len(syms))
	for _, s := range syms {
		strs = append(strs, s.String())
	}
	b, err2 := json.Marshal(strs)
	if err2 == nil {
		_ = r.redis.Set(ctx, r.key, string(b), r.ttl) // ignore set error, fallback is fine
	}
	return syms, nil
}
