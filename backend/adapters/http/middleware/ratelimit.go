package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// keyLimiterMinTTL is the floor for how long a per-key limiter is kept
// after its last request before it's eligible for eviction — see
// safeEvictionTTL, which is what actually determines the TTL used.
const keyLimiterMinTTL = 10 * time.Minute

// safeEvictionTTL returns how long an idle key's rate.Limiter can be
// evicted after without granting a free quota reset — see PR-075 CR
// follow-up (Issue 1). Deleting a limiter entry and recreating it on the
// next request starts that new limiter at a full burst, same as a brand
// new key. That's only *observably* equivalent to leaving the real
// limiter in place if the real one would also have refilled to a full
// burst by then — i.e. the TTL must be at least burst/perSecond, the time
// a fully-drained bucket takes to refill.
//
// This mattered concretely for PerUserRateLimit(5, 3) (5/hour, burst 3):
// natural refill time is burst/perSecond = 3/(5/3600) = 2160s = 36
// minutes, but the old shared 10-minute TTL evicted (and therefore
// refilled) the limiter more than 3x faster than that — a user idle for
// just over 10 minutes between bursts got a free full burst again well
// before the real 5/hour rate would have allowed it, permitting
// materially more than 5 calls/hour by spacing bursts >10 minutes apart.
// PerIPRateLimit's typical rates (e.g. 60/min, burst 3-10) refill in
// single-digit seconds, so keyLimiterMinTTL was already safe there — this
// only changes behavior for callers whose natural refill time exceeds it.
func safeEvictionTTL(perSecond float64, burst int) time.Duration {
	if perSecond <= 0 {
		return keyLimiterMinTTL
	}
	refill := time.Duration(float64(burst) / perSecond * float64(time.Second))
	if refill < keyLimiterMinTTL {
		return keyLimiterMinTTL
	}
	return refill
}

// keyLimiterCleanupEvery triggers an opportunistic sweep of stale entries
// every N requests, instead of a background goroutine — these middlewares
// guard low-traffic endpoints, so a periodic scan on the request path is
// simpler than managing a ticker's lifecycle for no measurable cost.
const keyLimiterCleanupEvery = 500

type keyLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// noKeySentinel is the bucket used for requests whose keyFunc can't derive
// a real key (e.g. PerUserRateLimit with no authenticated user in
// context). Never returned by a real IP or user ID, so it can't collide
// with one.
//
// This exists so "no key" degrades to "one shared, still-enforced bucket"
// rather than "unlimited" — see PR-075 CR follow-up: PerUserRateLimit is
// only a real per-user limit when it runs behind auth that has already
// populated the user ID; nothing at compile time stops a future route from
// wiring it behind log-only auth instead, and unauthenticated traffic is
// exactly the traffic most likely to abuse a costly endpoint. A shared
// bucket for the "couldn't identify the caller" case means that
// misconfiguration still gets *some* rate limiting (a single pool shared
// by all such requests) instead of none — it does not, on its own, make
// wiring `PerUserRateLimit` behind log-only auth into a correct per-user
// limit; nothing short of code review catches that.
const noKeySentinel = "\x00__no_key__"

// perKeyRateLimit is the shared TTL-bounded, per-key rate limiter behind
// both PerIPRateLimit and PerUserRateLimit (PR-075) — one map, one eviction
// sweep, one Allow/429 path, parameterized only by how the key is derived
// from the request and the allowed rate. perSecond is taken as an explicit
// requests-per-second rate (not per-minute or per-hour) precisely so this
// function never has to know or guess which "per-X" unit a caller is
// thinking in — each public wrapper below converts its own natural unit to
// per-second once, at its own call site, instead of this function silently
// assuming everyone means the same "per minute" convention it used to (a
// caller that wanted per-hour had to pre-divide by 60 before calling in,
// producing a value that was neither per-hour nor per-minute — an easy
// mistake to repeat for the next caller with no compiler or runtime signal
// that it was wrong; see PR-075 CR follow-up).
//
// keyFunc returning "" means "no key to limit on for this request" (e.g.
// no authenticated user yet) — see noKeySentinel: such requests all share
// one bucket rather than bypassing the limiter.
func perKeyRateLimit(keyFunc func(*http.Request) string, perSecond float64, burst int) func(http.Handler) http.Handler {
	return perKeyRateLimitWithClock(keyFunc, perSecond, burst, time.Now)
}

// perKeyRateLimitWithClock is perKeyRateLimit with an injectable clock,
// used only for this package's own eviction/sweep bookkeeping (which key
// is stale enough to evict) — never for the underlying rate.Limiter's own
// token accounting, which always uses real wall-clock time internally
// regardless of nowFn (golang.org/x/time/rate has no clock injection of
// its own). That split is enough to deterministically test the eviction
// question ("did the CR-follow-up TTL fix actually prevent a premature
// reset") without needing to fake the limiter's own timing too: a freshly
// created rate.Limiter grants a full burst immediately in real time, so a
// test just needs to control when eviction fires, not the token math
// itself. See ratelimit_internal_test.go.
func perKeyRateLimitWithClock(keyFunc func(*http.Request) string, perSecond float64, burst int, nowFn func() time.Time) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*keyLimiterEntry)
	rps := rate.Limit(perSecond)
	ttl := safeEvictionTTL(perSecond, burst)
	var requestCount uint64
	var lastSweep time.Time

	limiterFor := func(key string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		now := nowFn()
		requestCount++
		// Sweep every keyLimiterCleanupEvery requests, OR opportunistically
		// once ttl has passed since the last sweep regardless of count — a
		// count-only trigger means a low-traffic caller (e.g.
		// PerUserRateLimit on a rarely-hit endpoint) might not reach the
		// threshold for a very long time, leaving stale entries around far
		// longer than the TTL implies even though the map's total size
		// stays bounded by distinct-key count either way (not a leak, just
		// a wider-than-intended staleness window — CR follow-up).
		if requestCount%keyLimiterCleanupEvery == 0 || now.Sub(lastSweep) > ttl {
			for k, e := range limiters {
				if now.Sub(e.lastSeen) > ttl {
					delete(limiters, k)
				}
			}
			lastSweep = now
		}

		e, ok := limiters[key]
		if !ok {
			e = &keyLimiterEntry{limiter: rate.NewLimiter(rps, burst)}
			limiters[key] = e
		}
		e.lastSeen = now
		return e.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if key == "" {
				key = noKeySentinel
			}
			if !limiterFor(key).Allow() {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PerIPRateLimit returns middleware that limits each remote IP to
// requestsPerMinute requests (with the given burst), rejecting excess
// requests with 429. Intended for small, cheap-to-guard public endpoints
// (e.g. /api/device/claim) that have no other abuse protection — not a
// substitute for a real edge/proxy rate limiter in front of the whole API.
//
// Per-IP state is bounded via TTL eviction (see safeEvictionTTL): entries
// unused for a while are dropped on an opportunistic sweep, so sustained
// traffic from many distinct IPs doesn't grow the map forever.
func PerIPRateLimit(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	return perKeyRateLimit(func(r *http.Request) string {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		return ip
	}, float64(requestsPerMinute)/60.0, burst)
}

// PerUserRateLimit returns middleware that limits each authenticated user
// to requestsPerHour requests (with the given burst), rejecting excess
// requests with 429 — see PR-075, added for /api/payments/verify, which has
// no other abuse protection and each call triggers a live provider API
// request. Expressed per-hour rather than per-minute like PerIPRateLimit:
// a payment-verification limit is naturally a handful per hour, which
// doesn't have a clean whole-number per-minute equivalent.
//
// Must be registered so it runs AFTER auth has already populated the user
// ID in context (i.e. wrap the inner handler with this, then wrap that with
// RequireAuth — not the other way around). This is NOT a substitute for
// that ordering: a request with no authenticated user in context shares
// one bucket with every other such request (noKeySentinel) rather than
// being rejected — rejecting unauthenticated requests is RequireAuth's
// job, not this middleware's — so wiring this behind log-only auth still
// yields a single shared allowance across all unauthenticated callers
// combined, not a real per-user limit.
func PerUserRateLimit(requestsPerHour int, burst int) func(http.Handler) http.Handler {
	return perKeyRateLimit(func(r *http.Request) string {
		userID, ok := UserIDFromContextOK(r.Context())
		if !ok {
			return ""
		}
		return userID
	}, float64(requestsPerHour)/3600.0, burst)
}
