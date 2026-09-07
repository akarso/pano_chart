package infra

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"pano_chart/backend/application/ports"
)

// futuresDataRedisClient is the minimal Get/Set surface this decorator
// needs — matches symbol_universe.RedisClient's shape so the same
// *symbol_universe.GoRedisClient instance wired elsewhere in main.go can be
// passed directly, without adding a second concrete Redis dependency.
type futuresDataRedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// RedisCachedFuturesData decorates a ports.FuturesDataPort with per-symbol
// Redis caching. Binance's underlying data updates on a ~5 minute cadence
// (the "period=5m" used by the open-interest/long-short endpoints — see
// BinanceFuturesClient), so a short TTL avoids re-fetching more often than
// the data itself actually changes, matching the caching pattern already
// used for volume/universe/candle data elsewhere in this codebase.
type RedisCachedFuturesData struct {
	next  ports.FuturesDataPort
	redis futuresDataRedisClient
	ttl   time.Duration
}

// NewRedisCachedFuturesData constructs the decorator.
func NewRedisCachedFuturesData(next ports.FuturesDataPort, redis futuresDataRedisClient, ttl time.Duration) *RedisCachedFuturesData {
	return &RedisCachedFuturesData{next: next, redis: redis, ttl: ttl}
}

// FundingRate implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) FundingRate(ctx context.Context, symbol string) (float64, error) {
	return r.cachedFloat(ctx, "futures:funding:"+symbol, func() (float64, error) {
		return r.next.FundingRate(ctx, symbol)
	})
}

// OpenInterestHistory implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) OpenInterestHistory(ctx context.Context, symbol string) ([]float64, error) {
	key := "futures:oi:" + symbol
	if cached, err := r.redis.Get(ctx, key); err == nil && cached != "" {
		var vals []float64
		if jerr := json.Unmarshal([]byte(cached), &vals); jerr == nil {
			return vals, nil
		}
		// else: treat as a cache miss
	}
	vals, err := r.next.OpenInterestHistory(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if b, jerr := json.Marshal(vals); jerr == nil {
		_ = r.redis.Set(ctx, key, string(b), r.ttl) // ignore set error, fallback is fine
	}
	return vals, nil
}

// LongShortRatio implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) LongShortRatio(ctx context.Context, symbol string) (float64, error) {
	return r.cachedFloat(ctx, "futures:longshort:"+symbol, func() (float64, error) {
		return r.next.LongShortRatio(ctx, symbol)
	})
}

// cachedFloat is the shared cache-get/fetch/cache-set path for the two
// single-float64 methods above. OpenInterestHistory needs its own
// JSON-array handling, so it isn't folded into this.
func (r *RedisCachedFuturesData) cachedFloat(ctx context.Context, key string, fetch func() (float64, error)) (float64, error) {
	if cached, err := r.redis.Get(ctx, key); err == nil && cached != "" {
		if v, perr := strconv.ParseFloat(cached, 64); perr == nil {
			return v, nil
		}
		// else: treat as a cache miss
	}
	v, err := fetch()
	if err != nil {
		return 0, err
	}
	_ = r.redis.Set(ctx, key, strconv.FormatFloat(v, 'f', -1, 64), r.ttl) // ignore set error, fallback is fine
	return v, nil
}
