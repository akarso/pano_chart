package infrastructure

import (
	"context"
	"sync/atomic"

	infra "pano_chart/backend/infrastructure"

	"golang.org/x/time/rate"
)

// RateLimiter controls the rate of outbound API requests using a token-bucket algorithm.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a rate limiter that allows tokensPerMinute requests per minute
// with a small burst to absorb short spikes.
func NewRateLimiter(tokensPerMinute int) *RateLimiter {
	rps := float64(tokensPerMinute) / 60.0
	burst := tokensPerMinute / 6
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// Acquire blocks until the rate limiter allows the next request.
func (r *RateLimiter) Acquire() {
	atomic.AddInt64(&infra.GlobalMetrics.TokenAcquires, 1)
	_ = r.limiter.Wait(context.Background())
}

// Release is a no-op retained for backward compatibility.
func (r *RateLimiter) Release() {}
