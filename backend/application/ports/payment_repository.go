package ports

import (
	"context"

	"pano_chart/backend/domain"
)

// PurchaseRepository persists purchase records.
type PurchaseRepository interface {
	// Save stores a purchase record and returns its assigned ID.
	Save(ctx context.Context, purchase domain.Purchase) (int64, error)

	// FindByTransactionID looks up a purchase by its external transaction ID
	// and provider.  Returns the purchase and true if found, or a zero value
	// and false otherwise.
	FindByTransactionID(ctx context.Context, provider, externalTransactionID string) (domain.Purchase, bool, error)
}

// SubscriptionRepository persists subscription state.
type SubscriptionRepository interface {
	// Upsert creates or updates the subscription for a user.
	Upsert(ctx context.Context, subscription domain.Subscription) error

	// FindByUserID returns the subscription for the given user.
	// Returns the subscription and true if found.
	FindByUserID(ctx context.Context, userID string) (domain.Subscription, bool, error)
}

// VerifiedPurchaseApplier applies a verified purchase to the purchase ledger
// and live entitlements in one atomic unit of work.
//
// The SQLite payment repository implements this. Unit-test fakes should too
// so ActivateSubscription exercises the same ownership rules.
type VerifiedPurchaseApplier interface {
	// ApplyVerifiedPurchase updates the existing purchase row's owner and
	// expiration to match result, upserts the caller's subscription, and
	// expires the purchase's current owner when that owner differs from
	// result.UserID(). The current owner MUST be read inside the write
	// transaction (not from a pre-tx snapshot) so concurrent rebinds cannot
	// leave a stale intermediate holder entitled.
	ApplyVerifiedPurchase(ctx context.Context, result domain.PaymentVerificationResult) error
}
