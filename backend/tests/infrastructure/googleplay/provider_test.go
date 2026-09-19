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

	"pano_chart/backend/application/ports"
	"pano_chart/backend/infrastructure/googleplay"
)

// validPurchaseV2JSON returns a subscriptionsv2-shaped JSON body for a
// single-line-item active subscription — matches the real Google Play
// Developer API's purchases.subscriptionsv2.get response shape.
func validPurchaseV2JSON(productID string, start, expiry time.Time) string {
	return fmt.Sprintf(`{
		"startTime": %q,
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": [
			{"productId": %q, "expiryTime": %q, "latestSuccessfulOrderId": "GPA.1234-5678-9012"}
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

func TestProvider_VerifyPurchase_EncodesSpecialCharactersInToken(t *testing.T) {
	// Google Play purchase tokens are opaque and often contain `/`, `+`,
	// and `=`. Those MUST stay inside the last path segment — a raw `/`
	// would make Google look up a different (non-existent) resource and
	// 404, which the app reports as "we could not verify your purchase."
	now := time.Now().UTC()
	start := now.Add(-time.Hour)
	expiry := now.Add(30 * 24 * time.Hour)
	token := "GPA/abc+def=ghi"

	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validPurchaseV2JSON("pano_pro_monthly", start, expiry)))
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "pano_pro_monthly",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), token, "user1")
	require.NoError(t, err)
	assert.True(t, result.Valid())
	// `/` must be percent-encoded so it is not parsed as another path
	// segment. `+` and `=` are legal in a path segment and PathEscape
	// leaves them alone.
	assert.Contains(t, gotURI, "/tokens/GPA%2Fabc+def=ghi")
	assert.NotContains(t, gotURI, "/tokens/GPA/")
}

func TestProvider_VerifyPurchase_FreeTrial(t *testing.T) {
	// v2 has no separate "trial" subscriptionState — a trial period reads
	// as SUBSCRIPTION_STATE_ACTIVE the same as a paid period.
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.trial-001"},
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
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(3 * 24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.grace-001"},
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
		"lineItems": []map[string]interface{}{
			{"productId": "pano_pro_monthly", "expiryTime": now.Add(20 * 24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.canceled-001"},
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
	assert.ErrorIs(t, err, ports.ErrInvalidPurchaseToken)
	assert.NotErrorIs(t, err, ports.ErrProviderUnavailable)
	assert.NotErrorIs(t, err, ports.ErrProviderRateLimited)
}

func TestProvider_VerifyPurchase_API5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
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
	assert.ErrorIs(t, err, ports.ErrProviderUnavailable)
	assert.False(t, result.Valid())
}

func TestProvider_VerifyPurchase_API429IsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate"}`))
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "sub",
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.ErrorIs(t, err, ports.ErrProviderRateLimited)
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
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": ""}, // not yet owned by the user
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
		"lineItems": []map[string]interface{}{
			{"productId": "some_other_product", "expiryTime": wrongExpiry.Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.other-001"},
			{"productId": "pano_pro_monthly", "expiryTime": rightExpiry.Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.multi-001"},
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
	// Confirms the txID comes from the *matched* line item, not index 0's.
	assert.Equal(t, "GPA.multi-001", result.ExternalTransactionID())
}

func TestProvider_VerifyPurchase_SingleLineItemMismatch_FallsBackWithWarning(t *testing.T) {
	// CR follow-up: the realistic misconfiguration case — a stale/wrong
	// GOOGLE_PLAY_SUBSCRIPTION_ID with exactly one line item in the
	// response (this app's normal shape). Today's behavior is to still
	// accept the one real item (rejecting an otherwise-real purchase over
	// an app-side config mismatch would be worse for the user), logging a
	// warning rather than silently doing nothing — this test pins that
	// actual behavior so a future change to it is deliberate, not
	// accidental.
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": []map[string]interface{}{
			{"productId": "some_other_stale_product_id", "expiryTime": expiry.Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.mismatch-001"},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "pano_pro_monthly", // does not match the response's productId
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	result, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	require.NoError(t, err)
	assert.True(t, result.Valid())
	assert.Equal(t, "some_other_stale_product_id", result.ProductID())
	assert.WithinDuration(t, expiry, result.ExpirationTime(), time.Second)
}

func TestProvider_VerifyPurchase_MultipleLineItemsNoMatch_Errors(t *testing.T) {
	// Distinct from the single-item case above: with more than one line
	// item and no match, there's no principled way to guess which one the
	// caller meant, so this must error rather than silently pick index 0.
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": []map[string]interface{}{
			{"productId": "product_a", "expiryTime": now.Add(24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.ambiguous-a"},
			{"productId": "product_b", "expiryTime": now.Add(48 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.ambiguous-b"},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := googleplay.NewProvider(googleplay.Config{
		PackageName:    "com.test.app",
		SubscriptionID: "pano_pro_monthly", // matches neither product_a nor product_b
		AccessToken:    "tok",
		BaseURL:        srv.URL,
	}, srv.Client())

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to guess")
}

func TestProvider_VerifyPurchase_PausedState_NotValid(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_PAUSED",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": now.Add(60 * 24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.paused-001"},
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

func TestProvider_VerifyPurchase_UnparseableExpiryTime_Errors(t *testing.T) {
	// CR follow-up: an unparseable timestamp must surface as a clear
	// error, not a silent zero-time that only manifests later as a
	// confusing "expiration_time cannot be before start_time" failure
	// somewhere downstream in domain.NewSubscription.
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": "not-a-real-timestamp", "latestSuccessfulOrderId": "GPA.badtime-001"},
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

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expiryTime")
}

func TestProvider_VerifyPurchase_UnparseableStartTime_Errors(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         "also-not-a-timestamp",
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
		"lineItems": []map[string]interface{}{
			{"productId": "sub", "expiryTime": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.badstart-001"},
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

	_, err := p.VerifyPurchase(context.Background(), "tok1", "u1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "startTime")
}

func TestProvider_VerifyPurchase_NoLineItems_Errors(t *testing.T) {
	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"startTime":         now.Format(time.RFC3339Nano),
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
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
