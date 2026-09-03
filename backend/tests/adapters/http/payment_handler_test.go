package http_test

import (
	"bytes"
	"context"
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
		"userId":        "user1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
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
		"userId":        "u1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/verify", bytes.NewReader(body))
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
