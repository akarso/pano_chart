package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"pano_chart/backend/adapters/http/middleware"
)

func TestPerIPRateLimit_AllowsWithinBurstThenRejects(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := middleware.PerIPRateLimit(60, 3)(next) // 1/sec, burst 3

	codes := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodPost, "/anything", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		codes = append(codes, w.Result().StatusCode)
	}

	// First `burst` (3) requests allowed, the 4th exceeds it.
	assert.Equal(t, []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusTooManyRequests}, codes)
}

func TestPerIPRateLimit_TracksIPsIndependently(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := middleware.PerIPRateLimit(60, 1)(next) // burst 1 — second request from the SAME ip must 429

	req1 := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req1.RemoteAddr = "203.0.113.10:1"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Result().StatusCode)

	// Different IP — must not be affected by the first IP's burst usage.
	req2 := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req2.RemoteAddr = "203.0.113.11:1"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Result().StatusCode)

	// Same as req1's IP again — burst of 1 already used, must reject.
	req3 := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req3.RemoteAddr = "203.0.113.10:2" // same host, different port
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Result().StatusCode)
}

func TestPerIPRateLimit_ManyDistinctIPs_CleanupSweepDoesNotBreakEnforcement(t *testing.T) {
	// Regression test for the unbounded-map fix: pushing well past the
	// opportunistic cleanup threshold (500 requests) from many distinct
	// IPs must not panic, and per-IP enforcement must still work correctly
	// for a fresh IP afterward.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := middleware.PerIPRateLimit(60, 1)(next)

	for i := 0; i < 600; i++ {
		req := httptest.NewRequest(http.MethodPost, "/anything", nil)
		req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1", i%256)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// A brand new IP must still get its own fresh burst allowance.
	req := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req.RemoteAddr = "192.0.2.99:1"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	// And that same IP's burst is now exhausted (limiter still enforcing).
	req2 := httptest.NewRequest(http.MethodPost, "/anything", nil)
	req2.RemoteAddr = "192.0.2.99:2"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Result().StatusCode)
}
