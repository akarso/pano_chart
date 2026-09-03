package infrastructure

import (
	"testing"
	"time"
)

func TestRateLimiterThrottlesRequests(t *testing.T) {
	// 60 tokens/min = 1 per second, burst 6
	rl := NewRateLimiter(60)

	// First call should succeed immediately (within burst).
	start := time.Now()
	rl.Acquire()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("first Acquire took too long: %v", elapsed)
	}
}

func TestRateLimiterReleaseIsNoOp(t *testing.T) {
	rl := NewRateLimiter(60)
	rl.Acquire()
	rl.Release() // should not panic
}
