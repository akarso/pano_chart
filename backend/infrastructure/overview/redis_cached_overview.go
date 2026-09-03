package overview

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RedisClient abstracts Redis operations needed by the cache decorator.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedOverview is a decorator that caches OverviewUseCase results in Redis.
// On cache hit, the cached response is returned without calling the underlying use case.
// On cache miss or Redis failure, the underlying use case is called and the result is cached.
// Redis failures never break the endpoint.
type RedisCachedOverview struct {
	next      usecases.OverviewUseCase
	redis     RedisClient
	ttl       time.Duration
	keyPrefix string
}

// NewRedisCachedOverview constructs the decorator.
func NewRedisCachedOverview(next usecases.OverviewUseCase, redis RedisClient, ttl time.Duration, keyPrefix string) *RedisCachedOverview {
	return &RedisCachedOverview{
		next:      next,
		redis:     redis,
		ttl:       ttl,
		keyPrefix: keyPrefix,
	}
}

// Execute implements OverviewUseCase.
func (r *RedisCachedOverview) Execute(ctx context.Context, req usecases.GetOverviewRequest) ([]usecases.OverviewResult, error) {
	key := r.buildKey(req)

	// 1. Attempt Redis GET
	cached, err := r.redis.Get(ctx, key)
	if err == nil && cached != "" {
		var items []cachedOverviewResult
		if unmarshalErr := json.Unmarshal([]byte(cached), &items); unmarshalErr == nil {
			out, convErr := fromCached(items)
			if convErr == nil {
				return out, nil
			}
			// Conversion error: treat as cache miss
			fmt.Printf("[RedisCachedOverview] cache conversion error for key %s: %v\n", key, convErr)
		} else {
			// Unmarshal error: treat as cache miss
			fmt.Printf("[RedisCachedOverview] cache unmarshal error for key %s: %v\n", key, unmarshalErr)
		}
	} else if err != nil {
		// Redis GET failure: log and fallback
		fmt.Printf("[RedisCachedOverview] redis GET error for key %s: %v\n", key, err)
	}

	// 2. Cache miss: call underlying use case
	results, ucErr := r.next.Execute(ctx, req)
	if ucErr != nil {
		// Do not cache errors
		return nil, ucErr
	}

	// 3. Serialize and store with TTL
	payload := toCached(results)
	b, marshalErr := json.Marshal(payload)
	if marshalErr == nil {
		if setErr := r.redis.Set(ctx, key, string(b), r.ttl); setErr != nil {
			// Redis SET failure: log but still return result
			fmt.Printf("[RedisCachedOverview] redis SET error for key %s: %v\n", key, setErr)
		}
	}

	return results, nil
}

// buildKey generates a cache key from request parameters.
// Format: {prefix}:{timeframe}:{limit}
func (r *RedisCachedOverview) buildKey(req usecases.GetOverviewRequest) string {
	return fmt.Sprintf("%s:%s:%d", r.keyPrefix, req.Timeframe.String(), req.Limit)
}

// cachedOverviewResult is the JSON-serializable form of OverviewResult.
type cachedOverviewResult struct {
	Symbol     string    `json:"symbol"`
	TotalScore float64   `json:"totalScore"`
	Sparkline  []float64 `json:"sparkline"`
}

// toCached converts domain results to serializable form.
func toCached(results []usecases.OverviewResult) []cachedOverviewResult {
	out := make([]cachedOverviewResult, len(results))
	for i, r := range results {
		out[i] = cachedOverviewResult{
			Symbol:     r.Symbol.String(),
			TotalScore: r.TotalScore,
			Sparkline:  r.Sparkline,
		}
	}
	return out
}

// fromCached converts serialized form back to domain results.
func fromCached(cached []cachedOverviewResult) ([]usecases.OverviewResult, error) {
	out := make([]usecases.OverviewResult, len(cached))
	for i, c := range cached {
		sym, err := domain.NewSymbol(c.Symbol)
		if err != nil {
			return nil, fmt.Errorf("invalid cached symbol %q: %w", c.Symbol, err)
		}
		out[i] = usecases.OverviewResult{
			Symbol:     sym,
			TotalScore: c.TotalScore,
			Sparkline:  c.Sparkline,
		}
	}
	return out, nil
}
