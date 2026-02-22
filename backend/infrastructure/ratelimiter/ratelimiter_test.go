package infrastructure

import (
	"testing"
	"time"
)

func TestRateLimiterBlocksWhenNoTokens(t *testing.T) {
	rl := NewRateLimiter(2)

	// Acquire two tokens (should not block)
	rl.Acquire()
	rl.Acquire()

	// Start a goroutine to release a token after 50ms
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		rl.Release()
		close(done)
	}()

	start := time.Now()
	rl.Acquire() // Should block until token is released
	elapsed := time.Since(start)

	if elapsed < 45*time.Millisecond {
		t.Errorf("Acquire did not block as expected, elapsed: %v", elapsed)
	}
	<-done
}
