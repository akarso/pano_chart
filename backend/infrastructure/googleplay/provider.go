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

// subscriptionPurchaseResponse models the relevant fields from the
// Google Play Developer API v3 subscriptions endpoint:
//
//	GET /androidpublisher/v3/applications/{pkg}/purchases/subscriptions/{sub}/tokens/{token}
//
// See: https://developers.google.com/android-publisher/api-ref/rest/v3/purchases.subscriptions
type subscriptionPurchaseResponse struct {
	// PaymentState: 0 = pending, 1 = received, 2 = free trial, 3 = deferred
	PaymentState int `json:"paymentState"`
	// ExpiryTimeMillis: milliseconds since epoch when the subscription expires.
	ExpiryTimeMillis string `json:"expiryTimeMillis"`
	// StartTimeMillis: milliseconds since epoch when the subscription started.
	StartTimeMillis string `json:"startTimeMillis"`
	// OrderId: unique order ID from Google (the external transaction ID).
	OrderId string `json:"orderId"`
	// AutoRenewing: true if the subscription will renew automatically.
	AutoRenewing bool `json:"autoRenewing"`
}

// Config carries the runtime parameters required to verify Google Play
// subscriptions.
type Config struct {
	// PackageName is the Android application package (e.g. "com.example.app").
	PackageName string
	// SubscriptionID is the product/plan identifier defined in
	// Google Play Console (e.g. "pano_pro_monthly").
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
		"%s/androidpublisher/v3/applications/%s/purchases/subscriptions/%s/tokens/%s",
		p.cfg.baseURL(),
		p.cfg.PackageName,
		p.cfg.SubscriptionID,
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

	var purchase subscriptionPurchaseResponse
	if err := json.Unmarshal(body, &purchase); err != nil {
		return domain.PaymentVerificationResult{}, fmt.Errorf("decoding response: %w", err)
	}

	// PaymentState == 1 (received) or 2 (free trial) count as valid.
	valid := purchase.PaymentState == 1 || purchase.PaymentState == 2

	startTime := parseMillisToUTC(purchase.StartTimeMillis)
	expiryTime := parseMillisToUTC(purchase.ExpiryTimeMillis)

	if !valid {
		res, _ := domain.NewPaymentVerificationResult(
			false, "google_play", "", "", "", time.Time{}, time.Time{},
		)
		return res, nil
	}

	txID := purchase.OrderId
	if txID == "" {
		txID = purchaseToken // fallback — some sandbox purchases omit orderId
	}

	return domain.NewPaymentVerificationResult(
		true,
		"google_play",
		txID,
		p.cfg.SubscriptionID,
		userID,
		startTime,
		expiryTime,
	)
}

// parseMillisToUTC converts a millisecond-epoch string to time.Time UTC.
func parseMillisToUTC(millis string) time.Time {
	var ms int64
	_, _ = fmt.Sscanf(millis, "%d", &ms)
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
