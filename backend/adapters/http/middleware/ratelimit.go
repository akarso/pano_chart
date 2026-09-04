package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiterTTL is how long a per-IP limiter is kept after its last request
// before it's eligible for eviction.
const ipLimiterTTL = 10 * time.Minute

// ipLimiterCleanupEvery triggers an opportunistic sweep of stale entries
// every N requests, instead of a background goroutine — this middleware
// guards one low-traffic endpoint, so a periodic scan on the request path
// is simpler than managing a ticker's lifecycle for no measurable cost.
const ipLimiterCleanupEvery = 500

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// PerIPRateLimit returns middleware that limits each remote IP to
// requestsPerMinute requests (with the given burst), rejecting excess
// requests with 429. Intended for small, cheap-to-guard public endpoints
// (e.g. /api/device/claim) that have no other abuse protection — not a
// substitute for a real edge/proxy rate limiter in front of the whole API.
//
// Per-IP state is bounded via TTL eviction (see ipLimiterTTL): entries
// unused for a while are dropped on an opportunistic sweep, so sustained
// traffic from many distinct IPs doesn't grow the map forever.
func PerIPRateLimit(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*ipLimiterEntry)
	rps := rate.Limit(float64(requestsPerMinute) / 60.0)
	var requestCount uint64

	limiterFor := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		requestCount++
		if requestCount%ipLimiterCleanupEvery == 0 {
			for k, e := range limiters {
				if now.Sub(e.lastSeen) > ipLimiterTTL {
					delete(limiters, k)
				}
			}
		}

		e, ok := limiters[ip]
		if !ok {
			e = &ipLimiterEntry{limiter: rate.NewLimiter(rps, burst)}
			limiters[ip] = e
		}
		e.lastSeen = now
		return e.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			if !limiterFor(ip).Allow() {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
