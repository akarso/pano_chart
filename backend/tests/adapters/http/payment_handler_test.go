package http_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// fakeRouteCredentialStore is a minimal ports.CredentialStore for driving
// requests through the real NewVerifyPurchaseRoute wiring.
type fakeRouteCredentialStore struct {
	byHash map[string]string
}

func (f *fakeRouteCredentialStore) SaveIfUserUnclaimed(_ context.Context, secretHash, userID string) (bool, error) {
	if f.byHash == nil {
		f.byHash = make(map[string]string)
	}
	f.byHash[secretHash] = userID
	return true, nil
}

func (f *fakeRouteCredentialStore) Lookup(_ context.Context, secretHash string) (string, bool, error) {
	userID, ok := f.byHash[secretHash]
	return userID, ok, nil
}

func routeHashOf(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// ---- Fakes for payment handlers ----

type fakeVerifyPurchaseUC struct {
	lastInput usecases.VerifyPurchaseInput
	err       error
}

func (f *fakeVerifyPurchaseUC) Execute(_ context.Context, input usecases.VerifyPurchaseInput) error {
	f.lastInput = input
	return f.err
}

type fakeSubscriptionService struct {
	sub   domain.Subscription
	found bool
	err   error
}

func (f *fakeSubscriptionService) ActivateSubscription(_ context.Context, _ domain.PaymentVerificationResult) error {
	return nil
}

func (f *fakeSubscriptionService) IsActive(_ context.Context, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if !f.found {
		return false, nil
	}
	return f.sub.IsActive(time.Now().UTC()), nil
}

func (f *fakeSubscriptionService) GetSubscription(_ context.Context, _ string) (domain.Subscription, bool, error) {
	return f.sub, f.found, f.err
}

// ---- Verify Purchase Handler Tests ----

func TestVerifyPurchaseHandler_Success(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	body, _ := json.Marshal(map[string]string{
		"provider":      "stripe",
		"purchaseToken": "tok_123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "ok", result["status"])

	assert.Equal(t, "stripe", uc.lastInput.Provider)
	assert.Equal(t, "tok_123", uc.lastInput.PurchaseToken)
	assert.Equal(t, "user1", uc.lastInput.UserID)
}

func TestVerifyPurchaseHandler_AuthenticatedContext_IgnoresBodyUserID(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	// Body claims to be "attacker", but the authenticated context says "victim" —
	// this is exactly the cross-account-activation hole PR-071 closes.
	body, _ := json.Marshal(map[string]string{
		"provider":      "google_play",
		"purchaseToken": "tok_stolen",
		"userId":        "attacker",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "victim"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "victim", uc.lastInput.UserID)
}

func TestVerifyPurchaseHandler_NoAuthContext_Panics(t *testing.T) {
	// Unlike every other PR-070 endpoint, this handler has NO
	// migration-window fallback to a client-supplied userId — a missing
	// verified identity here must fail loudly (panic → net/http turns it
	// into a 500 for this one request), not silently trust the body.
	uc := &fakeVerifyPurchaseUC{}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	body, _ := json.Marshal(map[string]string{
		"provider":      "stripe",
		"purchaseToken": "tok_123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	w := httptest.NewRecorder()

	assert.Panics(t, func() { handler.ServeHTTP(w, req) })
}

// ---- Verify Purchase Route Tests (real production wiring) ----
//
// These drive requests through NewVerifyPurchaseRoute — the exact
// constructor cmd/api/main.go calls — instead of the bare handler, so a
// regression like "someone stops wrapping this route in auth" (which
// already happened once before PR-071) gets caught here rather than only
// in main.go's untested wiring.

func TestVerifyPurchaseRoute_Unauthenticated_401_EvenInLogOnlyMode(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	store := &fakeRouteCredentialStore{}
	route := adhttp.NewVerifyPurchaseRoute(uc, store)

	body, _ := json.Marshal(map[string]string{
		"provider":      "google_play",
		"purchaseToken": "tok_cheap",
		"userId":        "victim", // legacy field some old client might still send
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	w := httptest.NewRecorder()

	route.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
	assert.Empty(t, uc.lastInput.UserID, "the use case must never have been invoked")
}

func TestVerifyPurchaseRoute_ValidSecret_Succeeds(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	store := &fakeRouteCredentialStore{byHash: map[string]string{routeHashOf("s3cr3t"): "user1"}}
	route := adhttp.NewVerifyPurchaseRoute(uc, store)

	body, _ := json.Marshal(map[string]string{
		"provider":      "google_play",
		"purchaseToken": "tok_real",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer s3cr3t")
	w := httptest.NewRecorder()

	route.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "user1", uc.lastInput.UserID)
}

// TestVerifyPurchaseRoute_RateLimited_Returns429 is the regression test for
// PR-075: /api/payments/verify had no rate limiting at all — each call
// triggers a live provider API request, an easy cost/abuse amplification
// vector even with auth in place. Drives requests through the real
// NewVerifyPurchaseRoute wiring (not the bare handler), same rationale as
// the auth tests above: this must catch a regression in main.go's wiring,
// not just in the middleware unit tests.
func TestVerifyPurchaseRoute_RateLimited_Returns429(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	store := &fakeRouteCredentialStore{byHash: map[string]string{routeHashOf("s3cr3t"): "user1"}}
	route := adhttp.NewVerifyPurchaseRoute(uc, store)

	doRequest := func() int {
		body, _ := json.Marshal(map[string]string{
			"provider":      "google_play",
			"purchaseToken": "tok_real",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer s3cr3t")
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)
		return w.Result().StatusCode
	}

	// Burst allowance (3 — intentionally less than the 5/hour limit, see
	// the constants' doc comment) must all succeed.
	for i := 0; i < 3; i++ {
		if code := doRequest(); code != http.StatusOK {
			t.Fatalf("request %d: expected 200 within the burst allowance, got %d", i+1, code)
		}
	}
	// The next one exceeds it.
	if code := doRequest(); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhausting the burst allowance, got %d", code)
	}
}

// TestVerifyPurchaseRoute_RateLimitTracksUsersIndependently confirms the
// limit is per-user, not global — a hammered user must not lock out
// everyone else.
func TestVerifyPurchaseRoute_RateLimitTracksUsersIndependently(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	store := &fakeRouteCredentialStore{byHash: map[string]string{
		routeHashOf("secret-a"): "user-a",
		routeHashOf("secret-b"): "user-b",
	}}
	route := adhttp.NewVerifyPurchaseRoute(uc, store)

	doRequest := func(secret string) int {
		body, _ := json.Marshal(map[string]string{
			"provider":      "google_play",
			"purchaseToken": "tok_real",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)
		return w.Result().StatusCode
	}

	for i := 0; i < 3; i++ {
		if code := doRequest("secret-a"); code != http.StatusOK {
			t.Fatalf("user-a request %d: expected 200, got %d", i+1, code)
		}
	}
	if code := doRequest("secret-a"); code != http.StatusTooManyRequests {
		t.Fatalf("expected user-a to be rate limited, got %d", code)
	}
	// user-b's own allowance is untouched by user-a's burst.
	if code := doRequest("secret-b"); code != http.StatusOK {
		t.Fatalf("expected user-b's independent allowance to still work, got %d", code)
	}
}

func TestVerifyPurchaseHandler_MethodNotAllowed(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	req := httptest.NewRequest(http.MethodGet, "/api/payments/verify", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

func TestVerifyPurchaseHandler_InvalidBody(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify",
		bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestVerifyPurchaseHandler_UseCaseError(t *testing.T) {
	uc := &fakeVerifyPurchaseUC{err: fmt.Errorf("provider not registered")}
	handler := adhttp.NewVerifyPurchaseHandler(uc)

	body, _ := json.Marshal(map[string]string{
		"provider":      "unknown",
		"purchaseToken": "tok",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "u1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

// ---- Subscription Status Handler Tests ----

func TestSubscriptionStatusHandler_Active(t *testing.T) {
	now := time.Now().UTC()
	sub := domain.NewSubscriptionUnsafe(
		"user1", "stripe", "premium",
		now, now.Add(30*24*time.Hour), now,
	)
	svc := &fakeSubscriptionService{sub: sub, found: true}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscription/status", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, true, result["active"])
	assert.NotEmpty(t, result["expires_at"])
}

func TestSubscriptionStatusHandler_NoSubscription(t *testing.T) {
	svc := &fakeSubscriptionService{found: false}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscription/status", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "nobody"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, false, result["active"])
}

func TestSubscriptionStatusHandler_NoAuthContext_NoLegacyParam_401(t *testing.T) {
	svc := &fakeSubscriptionService{}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	// Simulates either a wiring bug, or RequireAuth running in log-only
	// mode against a request with no verified secret AND no legacy
	// ?userId= fallback — must reject, not silently proceed with "".
	req := httptest.NewRequest(http.MethodGet, "/api/subscription/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

func TestSubscriptionStatusHandler_LogOnlyMigrationFallback_UsesLegacyQueryParam(t *testing.T) {
	now := time.Now().UTC()
	sub := domain.NewSubscriptionUnsafe("legacy-user", "stripe", "premium", now, now.Add(time.Hour), now)
	svc := &fakeSubscriptionService{sub: sub, found: true}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	// No auth context (as RequireAuth would leave it in log-only mode for
	// a pre-PR-070 client), but the client still sends the old ?userId=.
	req := httptest.NewRequest(http.MethodGet, "/api/subscription/status?userId=legacy-user", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, true, result["active"])
}

func TestSubscriptionStatusHandler_MethodNotAllowed(t *testing.T) {
	svc := &fakeSubscriptionService{}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/subscription/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

func TestSubscriptionStatusHandler_ServiceError(t *testing.T) {
	svc := &fakeSubscriptionService{err: fmt.Errorf("db down")}
	handler := adhttp.NewSubscriptionStatusHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/subscription/status", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "u1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}
