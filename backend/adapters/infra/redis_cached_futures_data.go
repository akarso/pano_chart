package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"pano_chart/backend/application/ports"
)

// maxCachedErrorBytes bounds how much of a fetch error's message gets
// written into the cached failure marker — matches BinanceFuturesClient's
// own cap on raw response bodies (maxErrorBodyBytes), so a future change
// there (or an unusually verbose wrapped error) can't grow the cached value
// unbounded.
const maxCachedErrorBytes = 512

// futuresDataRedisClient is the minimal Get/Set surface this decorator
// needs — matches symbol_universe.RedisClient's shape so the same
// *symbol_universe.GoRedisClient instance wired elsewhere in main.go can be
// passed directly, without adding a second concrete Redis dependency.
type futuresDataRedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

// failureMarkerPrefix distinguishes a cached "the underlying fetch failed"
// entry from a cached success value that happens to be an empty string —
// see cachedFetch's doc.
const failureMarkerPrefix = "\x00ERR:"

// RedisCachedFuturesData decorates a ports.FuturesDataPort with per-symbol
// Redis caching. Binance's underlying data updates on a ~5 minute cadence
// (the "period=5m" used by the open-interest/long-short endpoints — see
// BinanceFuturesClient), so a short TTL avoids re-fetching more often than
// the data itself actually changes, matching the caching pattern already
// used for volume/universe/candle data elsewhere in this codebase.
//
// Failures are cached too, under a separate short failureTTL — CR follow-up,
// PR-081: without this, a symbol with no futures market (or Binance simply
// being down) was re-fetched on every single request with no backoff,
// repeatedly burning request weight against Binance under load with nothing
// to stop it hitting their rate-limit/ban thresholds.
type RedisCachedFuturesData struct {
	next       ports.FuturesDataPort
	redis      futuresDataRedisClient
	ttl        time.Duration
	failureTTL time.Duration
}

// NewRedisCachedFuturesData constructs the decorator. ttl bounds how long a
// successful fetch is cached; failureTTL bounds how long a failed one is —
// deliberately much shorter than ttl, so a transient Binance outage doesn't
// keep reporting "no data" long after Binance itself has recovered.
func NewRedisCachedFuturesData(next ports.FuturesDataPort, redis futuresDataRedisClient, ttl, failureTTL time.Duration) *RedisCachedFuturesData {
	return &RedisCachedFuturesData{next: next, redis: redis, ttl: ttl, failureTTL: failureTTL}
}

// FundingRate implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) FundingRate(ctx context.Context, symbol string) (float64, error) {
	raw, err := r.cachedFetch(ctx, "futures:funding:"+symbol, func() (string, error) {
		v, err := r.next.FundingRate(ctx, symbol)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	})
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseFloat(raw, 64)
	if perr != nil {
		// Cached value was corrupt (shouldn't happen — we wrote it
		// ourselves) — treat like any other fetch failure rather than
		// panicking or silently returning 0 as if it were a real rate.
		return 0, fmt.Errorf("futures:funding: corrupt cached value %q: %w", raw, perr)
	}
	return v, nil
}

// OpenInterestHistory implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) OpenInterestHistory(ctx context.Context, symbol string) ([]float64, error) {
	raw, err := r.cachedFetch(ctx, "futures:oi:"+symbol, func() (string, error) {
		vals, err := r.next.OpenInterestHistory(ctx, symbol)
		if err != nil {
			return "", err
		}
		b, jerr := json.Marshal(vals)
		if jerr != nil {
			return "", jerr
		}
		return string(b), nil
	})
	if err != nil {
		return nil, err
	}
	var vals []float64
	if jerr := json.Unmarshal([]byte(raw), &vals); jerr != nil {
		return nil, fmt.Errorf("futures:oi: corrupt cached value: %w", jerr)
	}
	return vals, nil
}

// LongShortRatio implements ports.FuturesDataPort.
func (r *RedisCachedFuturesData) LongShortRatio(ctx context.Context, symbol string) (float64, error) {
	raw, err := r.cachedFetch(ctx, "futures:longshort:"+symbol, func() (string, error) {
		v, err := r.next.LongShortRatio(ctx, symbol)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	})
	if err != nil {
		return 0, err
	}
	v, perr := strconv.ParseFloat(raw, 64)
	if perr != nil {
		return 0, fmt.Errorf("futures:longshort: corrupt cached value %q: %w", raw, perr)
	}
	return v, nil
}

// cachedFetch is the shared cache-get/fetch/cache-set path for all three
// methods above: a cache hit for a previously-successful fetch returns its
// stored string as-is; a cache hit for a previously-failed fetch
// (failureMarkerPrefix) returns a synthesized error without calling fetch
// again; a cache miss calls fetch, caching the result either way (success
// under ttl, failure under the shorter failureTTL) so a known-bad symbol
// isn't re-fetched on every single request.
//
// A fetch error is cached as symbol-level unavailability ONLY when it's a
// CONFIRMED, symbol-specific "no data" signal (*ErrSymbolDataUnavailable —
// see BinanceFuturesClient.getJSON's doc for exactly which cases qualify:
// a structured Binance API error, or an HTTP 200 with an empty/insufficient
// result set). Two categories of error are deliberately never cached:
//
//   - ctx itself being done (context.Canceled/context.DeadlineExceeded) —
//     CR follow-up, PR-081: BinanceFuturesDataProvider.Get runs its three
//     futures calls concurrently via errgroup.WithContext, which cancels
//     the shared context the instant ANY sibling call (including the
//     unrelated candle fetch) errors. An otherwise-healthy in-flight call
//     aborted that way returns context.Canceled, not a real "Binance
//     rejected/lacks this symbol" signal.
//   - any other transient/infrastructure failure — a network error, a
//     429/5xx or proxy/WAF response that doesn't match Binance's own error
//     shape, a malformed response body — CR follow-up: these say nothing
//     about whether the symbol itself is valid, so caching them the same
//     way would keep every request for that symbol reporting "unavailable"
//     for failureTTL long after Binance itself recovered.
//
// Either way, caching a false negative poisons every request for that
// symbol during failureTTL, not just the one that hit the transient issue.
func (r *RedisCachedFuturesData) cachedFetch(ctx context.Context, key string, fetch func() (string, error)) (string, error) {
	if cached, err := r.redis.Get(ctx, key); err == nil && cached != "" {
		if reason, isFailure := strings.CutPrefix(cached, failureMarkerPrefix); isFailure {
			return "", fmt.Errorf("futures data unavailable (cached failure): %s", reason)
		}
		return cached, nil
	}

	value, err := fetch()
	if err != nil {
		if ctx.Err() != nil {
			// Aborted because our own ctx is done, not because the fetch
			// itself was confirmed to fail — propagate without caching.
			return "", err
		}
		var unavailable *ErrSymbolDataUnavailable
		if !errors.As(err, &unavailable) {
			// Not a confirmed "Binance says this symbol/data doesn't
			// exist" signal — a transient/infrastructure failure.
			// Propagate without caching so the next request gets a fresh
			// attempt instead of a stale negative result.
			return "", err
		}
		msg := err.Error()
		if len(msg) > maxCachedErrorBytes {
			msg = msg[:maxCachedErrorBytes]
		}
		r.setCached(ctx, key, failureMarkerPrefix+msg, r.failureTTL)
		return "", err
	}
	r.setCached(ctx, key, value, r.ttl)
	return value, nil
}

// setCached writes to Redis, logging (not failing) on error — a cache
// write is best-effort, but a persistently broken write path (e.g. a wrong
// Redis ACL) should be visible in logs rather than silently degrading to
// "always a cache miss" forever with no signal — CR follow-up, PR-081.
func (r *RedisCachedFuturesData) setCached(ctx context.Context, key, value string, ttl time.Duration) {
	if err := r.redis.Set(ctx, key, value, ttl); err != nil {
		log.Printf("[futures-cache] failed to cache %s: %v", key, err)
	}
}
