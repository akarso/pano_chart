package googleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"pano_chart/backend/domain"
)

// subscriptionPurchaseV2Response models the relevant fields from the
// Google Play Developer API's subscriptionsv2 resource:
//
//	GET /androidpublisher/v3/applications/{pkg}/purchases/subscriptionsv2/tokens/{token}
//
// This replaces purchases.subscriptions.get (the "v3" flat
// paymentState/expiryTimeMillis shape) — Google deprecated that method in
// November 2023 in favor of this one; the old endpoint no longer reliably
// returns real data — see
// https://developers.google.com/android-publisher/api-ref/rest/v3/purchases.subscriptionsv2
//
// Unlike the old endpoint, the request does not name a specific
// subscription/product ID — the token itself determines what it covers,
// and the response lists every product line item associated with it.
type subscriptionPurchaseV2Response struct {
	// StartTime: RFC 3339 timestamp string for when the subscription started.
	StartTime string `json:"startTime"`
	// SubscriptionState: the top-level lifecycle state — see
	// subscriptionStateGrantsAccess's doc for which values this treats as
	// a real, verifiable subscription.
	SubscriptionState string `json:"subscriptionState"`
	// LatestOrderId: unique order ID from Google (the external transaction ID).
	LatestOrderID string `json:"latestOrderId"`
	// LineItems: one entry per product covered by this purchase — in
	// practice always exactly one for this app (a single subscription
	// product), but the API always returns an array.
	LineItems []subscriptionV2LineItem `json:"lineItems"`
}

// subscriptionV2LineItem is one entry of subscriptionPurchaseV2Response.LineItems.
type subscriptionV2LineItem struct {
	ProductID string `json:"productId"`
	// ExpiryTime: RFC 3339 timestamp string for when this line item's
	// access expires.
	ExpiryTime string `json:"expiryTime"`
}

// subscriptionStateGrantsAccess reports whether state represents a real,
// verifiable subscription purchase worth recording — as opposed to one
// that never completed (PENDING) or has definitively ended (EXPIRED).
//
//   - SUBSCRIPTION_STATE_ACTIVE: normal paid or free-trial period — v2 has
//     no separate "trial" state the way the old paymentState=2 did; a
//     trial reads as ACTIVE here too.
//   - SUBSCRIPTION_STATE_IN_GRACE_PERIOD: a renewal payment failed but
//     Google is still retrying — the user still has access during this
//     window, so this counts too.
//   - SUBSCRIPTION_STATE_CANCELED: auto-renew was turned off, but the
//     already-paid-for period hasn't ended yet — still a real, currently
//     accessible subscription until its own ExpiryTime.
//
// Deliberately excluded: PENDING (payment not yet completed — matches the
// old code's exclusion of paymentState=0), ON_HOLD (payment failed, access
// currently suspended), PAUSED, and EXPIRED. Whether "valid" here actually
// grants access *right now* is decided separately, downstream, by
// domain.Subscription.IsActive(now) comparing against ExpiryTime — this
// function only decides whether the API gave us a real subscription record
// worth storing at all.
func subscriptionStateGrantsAccess(state string) bool {
	switch state {
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD", "SUBSCRIPTION_STATE_CANCELED":
		return true
	default:
		return false
	}
}

// Config carries the runtime parameters required to verify Google Play
// subscriptions.
type Config struct {
	// PackageName is the Android application package (e.g. "com.example.app").
	PackageName string
	// SubscriptionID is the product/plan identifier defined in
	// Google Play Console (e.g. "pano_pro_monthly"). Used to pick the
	// matching line item out of the (usually single-entry) response, and
	// as a fallback ProductID if no line item matches by ID.
	SubscriptionID string
	// AccessToken is a valid OAuth2 access token for the Google Play
	// Developer API.  When empty the provider assumes the supplied
	// http.Client already carries credentials (e.g. service-account
	// transport).
	AccessToken string
	// ServiceAccountJSONPath is the file path to a Google service
	// account JSON key file.  When set, cmd/api/main.go creates an
	// auto-refreshing OAuth2 client and passes it to NewProvider,
	// leaving AccessToken empty.
	ServiceAccountJSONPath string
	// BaseURL overrides the API host (useful for testing).
	// Defaults to "https://androidpublisher.googleapis.com" if empty.
	BaseURL string
}

func (c Config) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://androidpublisher.googleapis.com"
}

// Provider implements ports.PaymentProviderPort for Google Play Billing.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider creates a Google Play payment provider.
func NewProvider(cfg Config, client *http.Client) *Provider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Provider{cfg: cfg, client: client}
}

// ProviderName returns "google_play".
func (p *Provider) ProviderName() string { return "google_play" }

// VerifyPurchase calls the Google Play Developer API to verify the
// purchase token and maps the response to a domain
// PaymentVerificationResult.
func (p *Provider) VerifyPurchase(
	ctx context.Context,
	purchaseToken string,
	userID string,
) (domain.PaymentVerificationResult, error) {
	url := fmt.Sprintf(
		"%s/androidpublisher/v3/applications/%s/purchases/subscriptionsv2/tokens/%s",
		p.cfg.baseURL(),
		p.cfg.PackageName,
		purchaseToken,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("building request: %w", err)
	}
	// When AccessToken is set, inject it manually.  Otherwise the
	// client's transport is expected to handle auth (service account).
	if p.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("google play API call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Return an invalid verification result — the purchase could not
		// be verified.
		invalid, _ := domain.NewPaymentVerificationResult(
			false, "google_play", "", "", "", time.Time{}, time.Time{},
		)
		return invalid, fmt.Errorf("google play API returned %d: %s", resp.StatusCode, string(body))
	}

	var purchase subscriptionPurchaseV2Response
	if err := json.Unmarshal(body, &purchase); err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("decoding response: %w", err)
	}

	if !subscriptionStateGrantsAccess(purchase.SubscriptionState) {
		res, _ := domain.NewPaymentVerificationResult(
			false, "google_play", "", "", "", time.Time{}, time.Time{},
		)
		return res, nil
	}

	// Pick the line item matching the configured product; fall back to the
	// first (and in practice only) entry if none match by ID exactly — a
	// token should only ever cover the product it was purchased for, but
	// being lenient here is safer than erroring out on an unexpected ID
	// formatting difference between Play Console and this config.
	item, ok := findLineItem(purchase.LineItems, p.cfg.SubscriptionID)
	if !ok {
		return domain.PaymentVerificationResult{}, fmt.Errorf("no line items in subscriptionsv2 response")
	}

	startTime := parseRFC3339UTC(purchase.StartTime)
	expiryTime := parseRFC3339UTC(item.ExpiryTime)

	txID := purchase.LatestOrderID
	if txID == "" {
		txID = purchaseToken // fallback — some sandbox purchases omit this
	}

	productID := item.ProductID
	if productID == "" {
		productID = p.cfg.SubscriptionID
	}

	return domain.NewPaymentVerificationResult(
		true,
		"google_play",
		txID,
		productID,
		userID,
		startTime,
		expiryTime,
	)
}

// findLineItem returns the line item whose ProductID matches want, or the
// first line item if none match (see VerifyPurchase's doc). ok is false
// only when items is empty entirely.
func findLineItem(items []subscriptionV2LineItem, want string) (subscriptionV2LineItem, bool) {
	for _, item := range items {
		if item.ProductID == want {
			return item, true
		}
	}
	if len(items) > 0 {
		return items[0], true
	}
	return subscriptionV2LineItem{}, false
}

// parseRFC3339UTC converts a Google API RFC 3339 timestamp string
// (e.g. "2026-09-06T13:38:04.126Z") to time.Time UTC. Returns the zero
// time for an empty or unparseable value.
func parseRFC3339UTC(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
