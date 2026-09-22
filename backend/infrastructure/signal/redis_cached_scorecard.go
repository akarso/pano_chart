package signal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	appsignal "pano_chart/backend/application/signal"
)

// ScorecardRedisClient is the Redis subset used by the scorecard cache.
type ScorecardRedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
}

const (
	scorecardCacheTTL       = 10 * time.Minute
	scorecardComputeTimeout = 8 * time.Second // under signal DB busy_timeout (15s)
)

// errScorecardCache marks a Redis failure already logged by redisGet*.
var errScorecardCache = errors.New("scorecard cache")

// ScorecardAPI is implemented by RedisCachedScorecard and ScorecardService.
type ScorecardAPI interface {
	Get(ctx context.Context, kind, label, tf, sinceRaw string) (appsignal.Scorecard, error)
	Summary(ctx context.Context, tf, sinceRaw string) (appsignal.SummaryResult, error)
}

// RedisCachedScorecard caches Get / Summary for 10 minutes (PR-092).
// Keys use a stable since bucket that matches the SQL bound (relative token or
// exact absolute RFC3339). Hit rate and baseline share one cached card / TTL.
//
// Shared flights use a refcounted work context: when the last waiter returns
// (including on cancel), the work context is canceled so an abandoned key
// releases the compute semaphore. Waiters still alive keep the flight alive
// under an 8s timeout. Redis hard errors fail the request (no SQLite fallback).
type RedisCachedScorecard struct {
	next       ScorecardAPI
	redis      ScorecardRedisClient
	ttl        time.Duration
	pfx        string
	computeSem chan struct{}

	flightsMu sync.Mutex
	flights   map[string]*scorecardFlight

	// onFlightJoin is optional; tests set it to observe waiter registration.
	onFlightJoin func()
	// onComputeWait is optional; tests set it when acquireCompute is about to block.
	onComputeWait func()
}

type scorecardFlight struct {
	mu      sync.Mutex
	waiters int
	cancel  context.CancelFunc
	done    chan struct{}
	val     interface{}
	err     error
	ready   bool // set under mu before close(done)
}

func (f *scorecardFlight) publish(val interface{}, err error) {
	f.mu.Lock()
	f.val = val
	f.err = err
	f.ready = true
	f.mu.Unlock()
}

func (f *scorecardFlight) result() (interface{}, error, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ready {
		return nil, nil, false
	}
	return f.val, f.err, true
}

// NewRedisCachedScorecard wraps next. redis may be nil (passthrough).
func NewRedisCachedScorecard(next ScorecardAPI, redis ScorecardRedisClient, keyPrefix string) *RedisCachedScorecard {
	if keyPrefix == "" {
		keyPrefix = "scorecards"
	}
	return &RedisCachedScorecard{
		next:       next,
		redis:      redis,
		ttl:        scorecardCacheTTL,
		pfx:        keyPrefix,
		computeSem: make(chan struct{}, 1),
		flights:    make(map[string]*scorecardFlight),
	}
}

// Get returns a cached or freshly computed scorecard.
func (c *RedisCachedScorecard) Get(ctx context.Context, kind, label, tf, sinceRaw string) (appsignal.Scorecard, error) {
	if c == nil || c.next == nil {
		return appsignal.Scorecard{}, fmt.Errorf("scorecard unavailable")
	}
	kindNorm, err := appsignal.NormalizeKind(kind)
	if err != nil {
		return appsignal.Scorecard{}, err
	}
	tfNorm, err := appsignal.NormalizeTimeframe(tf)
	if err != nil {
		return appsignal.Scorecard{}, err
	}
	label = strings.TrimSpace(label)
	win, err := appsignal.ResolveSince(sinceRaw, time.Now().UTC())
	if err != nil {
		return appsignal.Scorecard{}, err
	}
	key := scorecardCacheKey(c.pfx, "get", kindNorm, label, tfNorm, win.CacheBucket)

	card, hit, err := c.redisGetCard(ctx, ctx, key)
	if err != nil {
		return appsignal.Scorecard{}, err
	}
	if hit {
		return card, nil
	}

	v, err := c.doShared(ctx, key, func(workCtx context.Context) (interface{}, error) {
		card, hit, err := c.redisGetCard(workCtx, ctx, key)
		if err != nil {
			return appsignal.Scorecard{}, err
		}
		if hit {
			return card, nil
		}
		if err := c.acquireCompute(workCtx); err != nil {
			return appsignal.Scorecard{}, err
		}
		defer c.releaseCompute()
		card, err = c.next.Get(workCtx, kindNorm, label, tfNorm, sinceRaw)
		if err != nil {
			return appsignal.Scorecard{}, err
		}
		c.store(workCtx, key, card)
		return card, nil
	})
	if err != nil {
		return appsignal.Scorecard{}, err
	}
	return v.(appsignal.Scorecard), nil
}

// Summary returns a cached or freshly computed summary (since frozen in payload).
func (c *RedisCachedScorecard) Summary(ctx context.Context, tf, sinceRaw string) (appsignal.SummaryResult, error) {
	if c == nil || c.next == nil {
		return appsignal.SummaryResult{}, fmt.Errorf("scorecard unavailable")
	}
	tfNorm, err := appsignal.NormalizeTimeframe(tf)
	if err != nil {
		return appsignal.SummaryResult{}, err
	}
	win, err := appsignal.ResolveSince(sinceRaw, time.Now().UTC())
	if err != nil {
		return appsignal.SummaryResult{}, err
	}
	key := scorecardCacheKey(c.pfx, "summary", "", "", tfNorm, win.CacheBucket)

	sum, hit, err := c.redisGetSummary(ctx, ctx, key)
	if err != nil {
		return appsignal.SummaryResult{}, err
	}
	if hit {
		return sum, nil
	}

	v, err := c.doShared(ctx, key, func(workCtx context.Context) (interface{}, error) {
		sum, hit, err := c.redisGetSummary(workCtx, ctx, key)
		if err != nil {
			return appsignal.SummaryResult{}, err
		}
		if hit {
			return sum, nil
		}
		if err := c.acquireCompute(workCtx); err != nil {
			return appsignal.SummaryResult{}, err
		}
		defer c.releaseCompute()
		sum, err = c.next.Summary(workCtx, tfNorm, sinceRaw)
		if err != nil {
			return appsignal.SummaryResult{}, err
		}
		c.store(workCtx, key, sum)
		return sum, nil
	})
	if err != nil {
		return appsignal.SummaryResult{}, err
	}
	return v.(appsignal.SummaryResult), nil
}

// doShared coalesces callers on key. The work context is canceled when the
// last waiter returns (abandon) or when the 8s timeout fires.
func (c *RedisCachedScorecard) doShared(
	ctx context.Context, key string, compute func(context.Context) (interface{}, error),
) (interface{}, error) {
	c.flightsMu.Lock()
	f, exists := c.flights[key]
	if exists && f.waiters > 0 {
		f.waiters++
		c.flightsMu.Unlock()
		if c.onFlightJoin != nil {
			c.onFlightJoin()
		}
	} else {
		workCtx, cancel := context.WithTimeout(context.Background(), scorecardComputeTimeout)
		f = &scorecardFlight{
			waiters: 1,
			cancel:  cancel,
			done:    make(chan struct{}),
		}
		c.flights[key] = f
		c.flightsMu.Unlock()
		if c.onFlightJoin != nil {
			c.onFlightJoin()
		}
		go c.runFlight(key, f, workCtx, cancel, compute)
	}

	select {
	case <-f.done:
		c.leaveFlight(f)
		val, err, _ := f.result()
		return val, err
	case <-ctx.Done():
		// Leave first so a sole waiter cancels the work context. Then wait
		// for publish — no default, so a finished card cannot lose to cancel.
		c.leaveFlight(f)
		<-f.done
		val, err, _ := f.result()
		return val, err
	}
}

func (c *RedisCachedScorecard) leaveFlight(f *scorecardFlight) {
	c.flightsMu.Lock()
	defer c.flightsMu.Unlock()
	f.waiters--
	if f.waiters <= 0 {
		f.waiters = 0
		if f.cancel != nil {
			f.cancel()
		}
	}
}

func (c *RedisCachedScorecard) runFlight(
	key string,
	f *scorecardFlight,
	workCtx context.Context,
	cancel context.CancelFunc,
	compute func(context.Context) (interface{}, error),
) {
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			// The compute closure releases the semaphore itself (defer after
			// a successful acquire). Do not release here — that would drop
			// another key's permit if it acquired in the gap.
			err := fmt.Errorf("scorecard: panic: %v", r)
			log.Printf("[scorecard-cache] compute %s: %v", key, err)
			f.publish(nil, err)
		}
		c.flightsMu.Lock()
		if c.flights[key] == f {
			delete(c.flights, key)
		}
		c.flightsMu.Unlock()
		close(f.done)
	}()
	val, err := compute(workCtx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errScorecardCache) {
		log.Printf("[scorecard-cache] compute %s: %v", key, err)
	}
	f.publish(val, err)
}

func (c *RedisCachedScorecard) acquireCompute(ctx context.Context) error {
	select {
	case c.computeSem <- struct{}{}:
		return nil
	default:
	}
	if c.onComputeWait != nil {
		c.onComputeWait()
	}
	select {
	case c.computeSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *RedisCachedScorecard) releaseCompute() {
	select {
	case <-c.computeSem:
	default:
	}
}

// redisGetCard calls Redis with getCtx. logCtx is the request context and is
// used only to decide whether a failure is logged.
func (c *RedisCachedScorecard) redisGetCard(getCtx, logCtx context.Context, key string) (appsignal.Scorecard, bool, error) {
	if c.redis == nil {
		return appsignal.Scorecard{}, false, nil
	}
	raw, err := c.redis.Get(getCtx, key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return appsignal.Scorecard{}, false, nil
		}
		return appsignal.Scorecard{}, false, wrapRedisGetErr(logCtx, key, err)
	}
	if raw == "" {
		return appsignal.Scorecard{}, false, nil
	}
	var card appsignal.Scorecard
	if json.Unmarshal([]byte(raw), &card) != nil {
		return appsignal.Scorecard{}, false, nil
	}
	return card, true, nil
}

func (c *RedisCachedScorecard) redisGetSummary(getCtx, logCtx context.Context, key string) (appsignal.SummaryResult, bool, error) {
	if c.redis == nil {
		return appsignal.SummaryResult{}, false, nil
	}
	raw, err := c.redis.Get(getCtx, key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return appsignal.SummaryResult{}, false, nil
		}
		return appsignal.SummaryResult{}, false, wrapRedisGetErr(logCtx, key, err)
	}
	if raw == "" {
		return appsignal.SummaryResult{}, false, nil
	}
	var res appsignal.SummaryResult
	if json.Unmarshal([]byte(raw), &res) != nil {
		return appsignal.SummaryResult{}, false, nil
	}
	return res, true, nil
}

func wrapRedisGetErr(reqCtx context.Context, key string, err error) error {
	// Log from the request context, not the flight's work context. A dial
	// timeout is DeadlineExceeded while the request is still live. Skip the
	// log only when the request itself is already canceled or expired.
	if reqCtx.Err() == nil {
		log.Printf("[scorecard-cache] GET %s: %v", key, err)
	}
	return fmt.Errorf("%w: %w", errScorecardCache, err)
}

func scorecardCacheKey(pfx, op, kind, label, tf, sinceBucket string) string {
	// Path-escape each field; '/' separators prevent colon-label collisions.
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		pfx, op,
		url.PathEscape(kind),
		url.PathEscape(label),
		url.PathEscape(tf),
		url.PathEscape(sinceBucket),
	)
}

func (c *RedisCachedScorecard) store(ctx context.Context, key string, v interface{}) {
	if c.redis == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("[scorecard-cache] marshal %s: %v", key, err)
		return
	}
	if err := c.redis.Set(ctx, key, string(b), c.ttl); err != nil {
		log.Printf("[scorecard-cache] SET %s: %v", key, err)
	}
}
