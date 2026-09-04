package middleware

import (
	"testing"
	"time"
)

// TestSafeEvictionTTL_CoversNaturalRefillTime is a direct, in-package unit
// test for the PR-075 CR blocker: evicting an idle key's rate.Limiter and
// recreating it on the next request grants a free full-burst reset, which
// is only safe once the real limiter would have refilled to full burst
// anyway. Concretely pins the PerUserRateLimit(5, 3) case (5/hour, burst
// 3) that was actually wrong before this fix: the old fixed 10-minute TTL
// let a user get a fresh burst every ~10 minutes instead of the intended
// ~36 minutes (burst/perSecond), permitting well over 5 calls/hour.
func TestSafeEvictionTTL_CoversNaturalRefillTime(t *testing.T) {
	perSecond := 5.0 / 3600.0 // PerUserRateLimit(5, ...) — 5/hour
	burst := 3

	got := safeEvictionTTL(perSecond, burst)
	wantMin := 36 * time.Minute // burst/perSecond, computed independently above

	if got < wantMin {
		t.Errorf("safeEvictionTTL(%v, %d) = %v, want >= %v (the real bucket's natural refill time) — "+
			"evicting sooner than this grants a free burst reset before the real limiter would have refilled",
			perSecond, burst, got, wantMin)
	}
}

// TestSafeEvictionTTL_FastRefillStaysAtTheFloor confirms the fix doesn't
// regress PerIPRateLimit's existing behavior: a fast-refilling limiter
// (natural refill well under keyLimiterMinTTL) still uses the floor, not a
// tiny TTL that would sweep entries far more aggressively than before.
func TestSafeEvictionTTL_FastRefillStaysAtTheFloor(t *testing.T) {
	perSecond := 60.0 / 60.0 // PerIPRateLimit(60, ...) — 1/sec
	burst := 3               // natural refill: 3 seconds

	got := safeEvictionTTL(perSecond, burst)
	if got != keyLimiterMinTTL {
		t.Errorf("expected the %v floor for a fast-refilling limiter, got %v", keyLimiterMinTTL, got)
	}
}

// TestSafeEvictionTTL_ZeroRateDoesNotDivideByZero guards the degenerate
// perSecond <= 0 case some future caller might construct (e.g. a
// misconfigured requestsPerHour of 0) — must return a sane floor, not
// panic, NaN, or an infinite TTL.
func TestSafeEvictionTTL_ZeroRateDoesNotDivideByZero(t *testing.T) {
	got := safeEvictionTTL(0, 5)
	if got != keyLimiterMinTTL {
		t.Errorf("expected the %v floor for a zero rate, got %v", keyLimiterMinTTL, got)
	}
}
