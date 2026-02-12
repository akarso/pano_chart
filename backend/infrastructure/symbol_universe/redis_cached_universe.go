package symbol_universe

import (
	"context"
	"encoding/json"
	"time"
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
			return out, nil
		}
		// else: treat as cache miss
	}
	// Cache miss or error: call next
	syms, err := r.next.Symbols(ctx)
	if err != nil {
		return nil, err
	}
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
