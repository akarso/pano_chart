package middleware

import (
	"net/http"
	"net/http/httptest"
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

// fixedKeyLimiter builds the same middleware perKeyRateLimit constructs
// (and that both PerIPRateLimit and PerUserRateLimit delegate to), but with
// an injectable clock and a single constant key — isolating the
// eviction/reuse behavior under test from request-derived key parsing,
// which is already covered elsewhere. Params match PerUserRateLimit's
// actual production call for /api/payments/verify (5/hour, burst 3), the
// exact case the CR finding is about.
func fixedKeyLimiter(nowFn func() time.Time) http.Handler {
	mw := perKeyRateLimitWithClock(func(*http.Request) string { return "user1" }, 5.0/3600.0, 3, nowFn)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func doRequest(t *testing.T, h http.Handler) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Code
}

// TestPerKeyRateLimit_NotEvictedBeforeTTL is the clock-controlled
// regression test the CR follow-up asked for: with a fixed key drained to
// its burst, advancing the injected clock to just under safeEvictionTTL
// must NOT evict the limiter entry — a still-exhausted key must keep
// rejecting requests, not silently get a free burst reset. Exercises the
// same perKeyRateLimitWithClock code path that both PerIPRateLimit and
// PerUserRateLimit build on (via perKeyRateLimit).
func TestPerKeyRateLimit_NotEvictedBeforeTTL(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	ttl := safeEvictionTTL(5.0/3600.0, 3)
	h := fixedKeyLimiter(func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if code := doRequest(t, h); code != http.StatusOK {
			t.Fatalf("burst request %d: got %d, want 200 (burst not yet exhausted)", i+1, code)
		}
	}
	if code := doRequest(t, h); code != http.StatusTooManyRequests {
		t.Fatalf("post-burst request: got %d, want 429 (burst exhausted)", code)
	}

	now = now.Add(ttl - time.Second)
	if code := doRequest(t, h); code != http.StatusTooManyRequests {
		t.Fatalf("request just under safeEvictionTTL (%v): got %d, want 429 — "+
			"the entry must not be evicted (and therefore reset) before its natural refill time", ttl, code)
	}
}

// TestPerKeyRateLimit_EvictedAndReusedAfterTTL is the companion case: once
// the injected clock has advanced past safeEvictionTTL since the key was
// last seen, the next request must evict the stale entry and transparently
// create a fresh one — indistinguishable, from the caller's perspective,
// from a brand-new key getting a full burst.
func TestPerKeyRateLimit_EvictedAndReusedAfterTTL(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	ttl := safeEvictionTTL(5.0/3600.0, 3)
	h := fixedKeyLimiter(func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if code := doRequest(t, h); code != http.StatusOK {
			t.Fatalf("burst request %d: got %d, want 200", i+1, code)
		}
	}
	if code := doRequest(t, h); code != http.StatusTooManyRequests {
		t.Fatalf("post-burst request: got %d, want 429", code)
	}

	now = now.Add(ttl + time.Second)
	for i := 0; i < 3; i++ {
		if code := doRequest(t, h); code != http.StatusOK {
			t.Fatalf("post-eviction burst request %d: got %d, want 200 — "+
				"the entry should have been evicted past safeEvictionTTL (%v) and recreated with a fresh burst", i+1, ttl, code)
		}
	}
	if code := doRequest(t, h); code != http.StatusTooManyRequests {
		t.Fatalf("post-eviction request past the fresh burst: got %d, want 429 (the new limiter still enforces the same burst)", code)
	}
}
