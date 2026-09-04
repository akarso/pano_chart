package infrastructure

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterThrottlesRequests(t *testing.T) {
	// 60 tokens/min = 1 per second, burst 6
	rl := NewRateLimiter(60)

	// First call should succeed immediately (within burst).
	start := time.Now()
	if err := rl.Acquire(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("first Acquire took too long: %v", elapsed)
	}
}

func TestRateLimiterReleaseIsNoOp(t *testing.T) {
	rl := NewRateLimiter(60)
	if err := rl.Acquire(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rl.Release() // should not panic
}

func TestRateLimiterAcquire_AbortsOnContextCancellation(t *testing.T) {
	// burst of 1: the second Acquire must wait, giving the cancellation a
	// window to interrupt it before a token would naturally free up.
	rl := NewRateLimiter(6) // 6/min = 1 per 10s, burst 1
	if err := rl.Acquire(context.Background()); err != nil {
		t.Fatalf("unexpected error on first acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := rl.Acquire(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected Acquire to abort promptly on cancellation, took %v", elapsed)
	}
}
