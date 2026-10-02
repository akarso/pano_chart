package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/replay"
)

type replayCredStore struct {
	byHash map[string]string
}

func (f *replayCredStore) SaveIfUserUnclaimed(_ context.Context, secretHash, userID string) (bool, error) {
	if f.byHash == nil {
		f.byHash = make(map[string]string)
	}
	f.byHash[secretHash] = userID
	return true, nil
}

func (f *replayCredStore) Lookup(_ context.Context, secretHash string) (string, bool, error) {
	userID, ok := f.byHash[secretHash]
	return userID, ok, nil
}

func replayHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

type replaySubs struct {
	active map[string]bool
	err    error
}

func (s *replaySubs) IsActive(_ context.Context, userID string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.active[userID], nil
}

func TestRequireReplayAccess_PassthroughWithoutAsOf(t *testing.T) {
	var hit bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		if _, ok := replay.AsOf(r.Context()); ok {
			t.Fatal("asOf should not be set")
		}
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RequireReplayAccess(&replayCredStore{}, &replaySubs{})(inner)
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?timeframe=1h", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !hit || w.Code != http.StatusOK {
		t.Fatalf("hit=%v code=%d", hit, w.Code)
	}
}

func TestRequireReplayAccess_InvalidAsOf(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})
	h := middleware.RequireReplayAccess(&replayCredStore{}, &replaySubs{})(inner)
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=abc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestRequireReplayAccess_RequiresAuthAndPro(t *testing.T) {
	creds := &replayCredStore{byHash: map[string]string{replayHash("secret"): "pro-user"}}
	subs := &replaySubs{active: map[string]bool{"pro-user": true, "free-user": false}}
	var gotAsOf time.Time
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asOf, ok := replay.AsOf(r.Context())
		if !ok {
			t.Fatal("missing asOf")
		}
		gotAsOf = asOf
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RequireReplayAccess(creds, subs)(inner)

	// No auth
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=1700000000", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: code=%d", w.Code)
	}

	// Free user
	creds.byHash[replayHash("free")] = "free-user"
	r = httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=1700000000", nil)
	r.Header.Set("Authorization", "Bearer free")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("free: code=%d", w.Code)
	}

	// Pro user
	r = httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=1700000000", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("pro: code=%d body=%s", w.Code, w.Body.String())
	}
	if gotAsOf.Unix() != 1700000000 {
		t.Fatalf("asOf=%v", gotAsOf)
	}
}

func TestRequireReplayAccess_SubscriptionCheckError503(t *testing.T) {
	creds := &replayCredStore{byHash: map[string]string{replayHash("secret"): "pro-user"}}
	subs := &replaySubs{err: fmt.Errorf("db down")}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	})
	h := middleware.RequireReplayAccess(creds, subs)(inner)
	r := httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=1700000000", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d want 503", w.Code)
	}
}

func TestRequireReplayAccess_RateLimitsPerUser(t *testing.T) {
	creds := &replayCredStore{byHash: map[string]string{replayHash("secret"): "pro-user"}}
	subs := &replaySubs{active: map[string]bool{"pro-user": true}}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RequireReplayAccess(creds, subs)(inner)

	ok := 0
	denied := 0
	for i := 0; i < middleware.ReplayRateLimitBurst+3; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/rankings?asOf=1700000000", nil)
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		switch w.Code {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			denied++
		default:
			t.Fatalf("unexpected code %d", w.Code)
		}
	}
	if ok != middleware.ReplayRateLimitBurst || denied == 0 {
		t.Fatalf("ok=%d denied=%d want ok=%d and some 429", ok, denied, middleware.ReplayRateLimitBurst)
	}
}
