package ports

import (
	"context"

	"pano_chart/backend/domain"
)

// PaymentProviderPort verifies a purchase token with an external payment
// provider and returns a normalized verification result.
type PaymentProviderPort interface {
	// VerifyPurchase validates a purchase token and returns the verification
	// result.  Implementations must map provider-specific responses to the
	// domain PaymentVerificationResult.
	VerifyPurchase(ctx context.Context, purchaseToken string, userID string) (domain.PaymentVerificationResult, error)

	// ProviderName returns the canonical name of this provider
	// (e.g. "google_play", "stripe", "solana").
	ProviderName() string
}
