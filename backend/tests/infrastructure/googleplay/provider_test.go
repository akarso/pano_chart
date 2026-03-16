package googleplay_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/infrastructure/googleplay"
)

// validPurchaseJSON returns a JSON body for a valid subscription purchase.
func validPurchaseJSON(startMs, expiryMs int64) string {
	return fmt.Sprintf(`{
		"paymentState": 1,
		"orderId": "GPA.1234-5678-9012",
		"startTimeMillis": "%d",
		"expiryTimeMillis": "%d",
		"autoRenewing": true
	}`, startMs, expiryMs)
}

func TestProvider_ProviderName(t *testing.T) {
	p := googleplay.NewProvider(googleplay.Config{}, nil)
	assert.Equal(t, "google_play", p.ProviderName())
}

func TestProvider_VerifyPurchase_Valid(t *testing.T) {
	now := time.Now().UTC()
	startMs := now.Add(-24 * time.Hour).UnixMilli()
	expiryMs := now.Add(30 * 24 * time.Hour).UnixMilli()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/androidpublisher/v3/applications/com.test.app/purchases/subscriptions/pano_pro_monthly/tokens/test_token")
		assert.Equal(t, "Bearer test_access_token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validPurchaseJSON(startMs, expiryMs)))
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "pano_pro_monthly",
		AccessToken:    "test_access_token",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "test_token", "user1")
	require.NoError(t, err)
	assert.True(t, result.Valid())
	assert.Equal(t, "google_play", result.Provider())
	assert.Equal(t, "GPA.1234-5678-9012", result.ExternalTransactionID())
	assert.Equal(t, "pano_pro_monthly", result.ProductID())
	assert.Equal(t, "user1", result.UserID())
	assert.False(t, result.PurchaseTime().IsZero())
	assert.False(t, result.ExpirationTime().IsZero())
}

func TestProvider_VerifyPurchase_FreeTrial(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"paymentState":     2, // free trial
		"orderId":          "GPA.trial-001",
		"startTimeMillis":  fmt.Sprintf("%d", now.Add(-time.Hour).UnixMilli()),
		"expiryTimeMillis": fmt.Sprintf("%d", now.Add(7*24*time.Hour).UnixMilli()),
		"autoRenewing":     true,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "pano_pro_monthly",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	require.NoError(t, err)
	assert.True(t, result.Valid())
}

func TestProvider_VerifyPurchase_PendingPayment(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"paymentState":     0, // pending
		"orderId":          "GPA.pending-001",
		"startTimeMillis":  fmt.Sprintf("%d", now.UnixMilli()),
		"expiryTimeMillis": fmt.Sprintf("%d", now.Add(time.Hour).UnixMilli()),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "sub",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	require.NoError(t, err)
	assert.False(t, result.Valid())
}

func TestProvider_VerifyPurchase_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "sub",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.False(t, result.Valid())
}

func TestProvider_VerifyPurchase_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "sub",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoding response")
}

func TestProvider_VerifyPurchase_NoOrderId_FallbackToToken(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"paymentState":     1,
		"orderId":          "", // empty — sandbox sometimes omits this
		"startTimeMillis":  fmt.Sprintf("%d", now.UnixMilli()),
		"expiryTimeMillis": fmt.Sprintf("%d", now.Add(30*24*time.Hour).UnixMilli()),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "sub",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "my_purchase_token", "u1")
	require.NoError(t, err)
	assert.True(t, result.Valid())
	assert.Equal(t, "my_purchase_token", result.ExternalTransactionID())
}
