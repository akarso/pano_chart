package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// keyLimiterTTL is how long a per-key limiter is kept after its last
// request before it's eligible for eviction.
const keyLimiterTTL = 10 * time.Minute

// keyLimiterCleanupEvery triggers an opportunistic sweep of stale entries
// every N requests, instead of a background goroutine — these middlewares
// guard low-traffic endpoints, so a periodic scan on the request path is
// simpler than managing a ticker's lifecycle for no measurable cost.
const keyLimiterCleanupEvery = 500

type keyLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// perKeyRateLimit is the shared TTL-bounded, per-key rate limiter behind
// both PerIPRateLimit and PerUserRateLimit (PR-075) — one map, one eviction
// sweep, one Allow/429 path, parameterized only by how the key is derived
// from the request. keyFunc returning "" means "no key to limit on for
// this request" (e.g. no authenticated user yet), in which case the
// request passes through unlimited — the caller is responsible for
// registering this behind whatever actually establishes that identity
// (e.g. RequireAuth for PerUserRateLimit).
func perKeyRateLimit(keyFunc func(*http.Request) string, requestsPerMinute float64, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*keyLimiterEntry)
	rps := rate.Limit(requestsPerMinute / 60.0)
	var requestCount uint64

	limiterFor := func(key string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		requestCount++
		if requestCount%keyLimiterCleanupEvery == 0 {
			for k, e := range limiters {
				if now.Sub(e.lastSeen) > keyLimiterTTL {
					delete(limiters, k)
				}
			}
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
				next.ServeHTTP(w, r)
				return
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
// Per-IP state is bounded via TTL eviction (see keyLimiterTTL): entries
// unused for a while are dropped on an opportunistic sweep, so sustained
// traffic from many distinct IPs doesn't grow the map forever.
func PerIPRateLimit(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	return perKeyRateLimit(func(r *http.Request) string {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		return ip
	}, float64(requestsPerMinute), burst)
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
// RequireAuth — not the other way around). A request with no authenticated
// user in context is passed through unlimited here rather than rejected;
// that's RequireAuth's job, not this middleware's.
func PerUserRateLimit(requestsPerHour int, burst int) func(http.Handler) http.Handler {
	return perKeyRateLimit(func(r *http.Request) string {
		userID, ok := UserIDFromContextOK(r.Context())
		if !ok {
			return ""
		}
		return userID
	}, float64(requestsPerHour)/60.0, burst)
}
