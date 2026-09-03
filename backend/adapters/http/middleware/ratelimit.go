package middleware

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// PerIPRateLimit returns middleware that limits each remote IP to
// requestsPerMinute requests (with the given burst), rejecting excess
// requests with 429. Intended for small, cheap-to-guard public endpoints
// (e.g. /api/device/claim) that have no other abuse protection — not a
// substitute for a real edge/proxy rate limiter in front of the whole API.
//
// Known limitation: per-IP limiter state is never evicted, so it grows
// unboundedly under sustained attack from many distinct IPs. Acceptable for
// a low-traffic single endpoint; revisit (LRU eviction, or move this in
// front of a proxy) if it's ever applied more broadly.
func PerIPRateLimit(requestsPerMinute int, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*rate.Limiter)
	rps := rate.Limit(float64(requestsPerMinute) / 60.0)

	limiterFor := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := limiters[ip]
		if !ok {
			l = rate.NewLimiter(rps, burst)
			limiters[ip] = l
		}
		return l
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
