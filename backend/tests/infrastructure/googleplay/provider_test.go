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

// validPurchaseV2JSON returns a subscriptionsv2-shaped JSON body for a
// single-line-item active subscription — matches the real Google Play
// Developer API's purchases.subscriptionsv2.get response shape.
func validPurchaseV2JSON(productID string, start, expiry time.Time) string {
	return fmt.Sprintf(`{
		"startTime": %q,
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"latestOrderId": "GPA.1234-5678-9012",
		"lineItems": [
			{"productId": %q, "expiryTime": %q}
		]
	}`, start.Format(time.RFC3339Nano), productID, expiry.Format(time.RFC3339Nano))
}

func TestProvider_ProviderName(t *testing.T) {
	p := googleplay.NewProvider(googleplay.Config{}, nil)
	assert.Equal(t, "google_play", p.ProviderName())
}

func TestProvider_VerifyPurchase_Valid(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)
	expiry := now.Add(30 * 24 * time.Hour)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// v2 URL has no subscription/product ID segment — the token alone
		// determines what it covers.
		assert.Contains(t, r.URL.Path, "/androidpublisher/v3/applications/com.test.app/purchases/subscriptionsv2/tokens/test_token")
		assert.Equal(t, "Bearer test_access_token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validPurchaseV2JSON("pano_pro_monthly", start, expiry)))
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
	assert.WithinDuration(t, expiry, result.ExpirationTime(), time.Second)
}

func TestProvider_VerifyPurchase_FreeTrial(t *testing.T) {
	// v2 has no separate "trial" subscriptionState — a trial period reads
	// as SUBSCRIPTION_STATE_ACTIVE the same as a paid period.
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"latestOrderId":     "GPA.trial-001",
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)},
		},
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

func TestProvider_VerifyPurchase_GracePeriod_StillValid(t *testing.T) {
	// A failed renewal payment still under Google's retry window keeps
	// access — must be treated as valid, matching real subscriber
	// expectations (not just a literal API-shape test).
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_IN_GRACE_PERIOD",
		"latestOrderId":     "GPA.grace-001",
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(3 * 24 * time.Hour).Format(time.RFC3339Nano)},
		},
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

func TestProvider_VerifyPurchase_CanceledButNotYetExpired_StillValid(t *testing.T) {
	// Auto-renew off, but the already-paid-for period hasn't ended —
	// still real access until ExpirationTime; downstream
	// domain.Subscription.IsActive(now) is what actually cuts access off
	// once expiry passes, not this layer.
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_CANCELED",
		"latestOrderId":     "GPA.canceled-001",
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(20 * 24 * time.Hour).Format(time.RFC3339Nano)},
		},
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
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_PENDING",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(time.Hour).Format(time.RFC3339Nano)},
		},
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

func TestProvider_VerifyPurchase_OnHold_NotValid(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-40 * 24 * time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ON_HOLD",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(-1 * time.Hour).Format(time.RFC3339Nano)},
		},
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

func TestProvider_VerifyPurchase_Expired_NotValid(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_EXPIRED",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)},
		},
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
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"latestOrderId":     "", // sandbox sometimes omits this
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano)},
		},
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

func TestProvider_VerifyPurchase_MultipleLineItems_MatchesConfiguredProduct(t *testing.T) {
	// Defensive case: if the response ever contains more than one line
	// item, the one matching Config.SubscriptionID must be picked, not
	// just the first in the array.
	now := time.Now().UTC()
	wrongExpiry := now.Add(365 * 24 * time.Hour)
	rightExpiry := now.Add(30 * 24 * time.Hour)
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"latestOrderId":     "GPA.multi-001",
		"lineItems": []map[string]interface{}{
			{"productId": "some_other_product", "expiryTime": wrongExpiry.Format(time.RFC3339Nano)},
			{"productId": "pano_pro_monthly", "expiryTime": rightExpiry.Format(time.RFC3339Nano)},
		},
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
	assert.Equal(t, "pano_pro_monthly", result.ProductID())
	assert.WithinDuration(t, rightExpiry, result.ExpirationTime(), time.Second)
}

func TestProvider_VerifyPurchase_NoLineItems_Errors(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"latestOrderId":     "GPA.empty-001",
		"lineItems":         []map[string]interface{}{},
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

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no line items")
}
