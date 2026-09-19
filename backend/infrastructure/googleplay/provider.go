package googleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pano_chart/backend/application/ports"
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
	// LatestSuccessfulOrderID: the order ID of the latest successful order
	// for *this* line item (the external transaction ID) — not present if
	// the item isn't owned by the user yet. CR follow-up: there is no
	// top-level order ID on subscriptionsv2's response at all (an earlier
	// version of this struct assumed one, "latestOrderId" — verified
	// against Google's live API Discovery schema that no such field
	// exists; it never populated, so every real verification silently fell
	// through to the purchaseToken fallback below).
	LatestSuccessfulOrderID string `json:"latestSuccessfulOrderId"`
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
	reqURL := subscriptionsv2URL(p.cfg.baseURL(), p.cfg.PackageName, purchaseToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
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
		return domain.PaymentVerificationResult{}, fmt.Errorf("%w: google play API call: %v", ports.ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Do not log or embed the response body — Google error pages can be
		// huge and may contain sensitive snippets. Status alone is enough
		// for classification and ops.
		log.Printf("[googleplay] subscriptionsv2 returned %d for pkg=%s",
			resp.StatusCode, p.cfg.PackageName)
		invalid, _ := domain.NewPaymentVerificationResult(
			false, "google_play", "", "", "", time.Time{}, time.Time{},
		)
		return invalid, wrapPlayHTTPError(resp.StatusCode)
	}

	var purchase subscriptionPurchaseV2Response
	if err := json.Unmarshal(body, &purchase); err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("decoding response: %w", err)
	}

	if !subscriptionStateGrantsAccess(purchase.SubscriptionState) {
		log.Printf("[googleplay] token does not grant access: state=%s pkg=%s",
			purchase.SubscriptionState, p.cfg.PackageName)
		res, _ := domain.NewPaymentVerificationResult(
			false, "google_play", "", "", "", time.Time{}, time.Time{},
		)
		return res, nil
	}

	// Pick the line item matching the configured product.
	item, err := findLineItem(purchase.LineItems, p.cfg.SubscriptionID)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("subscriptionsv2 line items: %w", err)
	}

	startTime, err := parseRFC3339UTC(purchase.StartTime)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("subscriptionsv2 startTime: %w", err)
	}
	expiryTime, err := parseRFC3339UTC(item.ExpiryTime)
	if err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("subscriptionsv2 expiryTime: %w", err)
	}

	txID := item.LatestSuccessfulOrderID
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

// findLineItem returns the line item whose ProductID matches want.
//
// If none match: with exactly one line item (this app's normal case — one
// subscription product), the mismatch most likely means a stale/wrong
// GOOGLE_PLAY_SUBSCRIPTION_ID rather than a real multi-product purchase —
// logged loudly so that config drift is visible in production instead of
// silently tolerated forever, but the one real item is still returned
// rather than failing outright: rejecting an otherwise-real purchase over
// an app-side config mismatch would hurt the user more than granting
// access under a logged-as-suspicious productID. With more than one item
// and no match, there's no principled way to guess which one the caller
// meant — CR follow-up: the old v3 code had the product ID in the request
// URL itself, so a wrong config would 404 immediately; v2's token-only
// request lost that free diagnostic, so this has to catch it explicitly
// instead of silently defaulting to index 0.
func findLineItem(items []subscriptionV2LineItem, want string) (subscriptionV2LineItem, error) {
	for _, item := range items {
		if item.ProductID == want {
			return item, nil
		}
	}
	switch len(items) {
	case 0:
		return subscriptionV2LineItem{}, fmt.Errorf("no line items in subscriptionsv2 response")
	case 1:
		log.Printf("[googleplay] WARNING: line item productId %q does not match configured SubscriptionID %q — check GOOGLE_PLAY_SUBSCRIPTION_ID for drift",
			items[0].ProductID, want)
		return items[0], nil
	default:
		return subscriptionV2LineItem{}, fmt.Errorf(
			"no line item matches configured SubscriptionID %q among %d line items — refusing to guess which one", want, len(items))
	}
}

// subscriptionsv2URL builds the purchases.subscriptionsv2.get URL.
//
// The purchase token is a path parameter, not a query value — Google Play
// tokens are opaque and routinely contain `/`, `+`, and `=`. Putting the
// raw token in the path makes the request hit the wrong resource (Google
// returns 404), which the client then surfaces as "we could not verify
// your purchase." Path-escape each segment so those characters stay
// inside the token, not as extra path components.
func subscriptionsv2URL(base, packageName, purchaseToken string) string {
	base = strings.TrimRight(base, "/")
	return base + "/androidpublisher/v3/applications/" +
		url.PathEscape(packageName) +
		"/purchases/subscriptionsv2/tokens/" +
		url.PathEscape(purchaseToken)
}

// wrapPlayHTTPError maps Google Play HTTP status codes onto ports sentinel
// errors so HTTP handlers can classify without scraping error text. The
// error carries only the status code — never the response body.
//
// 401/403 are provider credential or API-permission failures, not proof the
// purchase token is bad — classify as unavailable so clients can retry once
// service credentials are fixed.
func wrapPlayHTTPError(status int) error {
	switch {
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: google play API returned %d", ports.ErrProviderRateLimited, status)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: google play API returned %d", ports.ErrProviderUnavailable, status)
	case status >= 500 || status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return fmt.Errorf("%w: google play API returned %d", ports.ErrProviderUnavailable, status)
	default:
		return fmt.Errorf("%w: google play API returned %d", ports.ErrInvalidPurchaseToken, status)
	}
}

// parseRFC3339UTC converts a Google API RFC 3339 timestamp string
// (e.g. "2026-09-06T13:38:04.126Z") to time.Time UTC. An empty or
// unparseable value is a real error, not a zero-time default — without
// this, a garbage timestamp could silently produce a technically-valid
// PaymentVerificationResult whose zero ExpirationTime only surfaces later
// as a confusing "expiration_time cannot be before start_time" error deep
// in domain.NewSubscription, instead of a clear parse-failure error right
// where the bad data was actually read — CR follow-up.
func parseRFC3339UTC(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing %q: %w", ts, err)
	}
	return t.UTC(), nil
}
