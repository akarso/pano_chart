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
