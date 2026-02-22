package infrastructure

import (
	"sync/atomic"
	infra "pano_chart/backend/infrastructure"
)

type RateLimiter struct {
	tokensPerMinute int
	bucket          chan struct{}
}

// NewRateLimiter creates a new RateLimiter with the given tokens per minute.
func NewRateLimiter(tokensPerMinute int) *RateLimiter {
	return &RateLimiter{
		tokensPerMinute: tokensPerMinute,
		bucket:          make(chan struct{}, tokensPerMinute),
	}
}

// Acquire blocks until a token is available.
func (r *RateLimiter) Acquire() {
	atomic.AddInt64(&infra.GlobalMetrics.TokenAcquires, 1)
	r.bucket <- struct{}{}
}

// Release returns a token to the bucket.
func (r *RateLimiter) Release() {
	<-r.bucket
}
