package usecases

import (
	"context"
	"fmt"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// SubscriptionService manages subscription lifecycle — activation,
// renewal, and status checks.  It is decoupled from any specific
// payment provider.
type SubscriptionService interface {
	// ActivateSubscription records a verified purchase and upserts the
	// user's subscription. Repeating the same provider transaction is
	// idempotent (restore / retry / Play Billing duplicate events) and
	// moves access to the calling user when a new device identity
	// presents a still-valid token. Returns an error if the verification
	// result is not valid.
	ActivateSubscription(ctx context.Context, result domain.PaymentVerificationResult) error

	// IsActive checks whether the user has an active (non-expired)
	// subscription.
	IsActive(ctx context.Context, userID string) (bool, error)

	// GetSubscription returns the subscription for the user, if any.
	GetSubscription(ctx context.Context, userID string) (domain.Subscription, bool, error)
}

type subscriptionService struct {
	purchases     ports.PurchaseRepository
	applier       ports.VerifiedPurchaseApplier
	subscriptions ports.SubscriptionRepository
}

// NewSubscriptionService constructs a SubscriptionService. purchases must
// implement ports.VerifiedPurchaseApplier (SQLite and test fakes do) so
// restore/rebind cannot silently skip ownership updates. Miswired adapters
// panic at construction, not on the first duplicate verify.
func NewSubscriptionService(
	purchases ports.PurchaseRepository,
	subscriptions ports.SubscriptionRepository,
) SubscriptionService {
	applier, ok := purchases.(ports.VerifiedPurchaseApplier)
	if !ok {
		panic("NewSubscriptionService: purchases must implement VerifiedPurchaseApplier")
	}
	return &subscriptionService{
		purchases:     purchases,
		applier:       applier,
		subscriptions: subscriptions,
	}
}

func (s *subscriptionService) ActivateSubscription(
	ctx context.Context,
	result domain.PaymentVerificationResult,
) error {
	if !result.Valid() {
		return fmt.Errorf("cannot activate subscription: verification result is not valid")
	}

	existing, exists, err := s.purchases.FindByTransactionID(
		ctx, result.Provider(), result.ExternalTransactionID(),
	)
	if err != nil {
		return fmt.Errorf("checking duplicate transaction: %w", err)
	}

	// Restore, retry, Play Billing duplicate stream events, and app
	// reinstall (which mints a new device user ID) all re-present a
	// transaction Google has already verified. Rejecting those as
	// "duplicate" is what the client surfaces as "we could not verify
	// your purchase" — after Google already took the money. Idempotently
	// refresh the caller's entitlement instead. If a different device
	// identity presents the same valid token, move access to that
	// identity: the token is proof of Play ownership; our user ID is
	// just a device credential.
	//
	// Purchase.user_id is the source of truth for who currently holds
	// the transaction. ApplyVerifiedPurchase expires that holder (not a
	// stale first owner), updates ownership + expiry, and upserts the
	// new subscription atomically.
	if exists {
		return s.applier.ApplyVerifiedPurchase(ctx, existing.UserID(), result)
	}

	purchase, err := domain.NewPurchase(
		result.UserID(),
		result.Provider(),
		result.ExternalTransactionID(),
		result.ProductID(),
		result.PurchaseTime(),
		result.ExpirationTime(),
		true,
	)
	if err != nil {
		return fmt.Errorf("creating purchase record: %w", err)
	}
	if _, err := s.purchases.Save(ctx, purchase); err != nil {
		return fmt.Errorf("saving purchase: %w", err)
	}

	return s.upsertSubscription(ctx, result)
}

func (s *subscriptionService) upsertSubscription(
	ctx context.Context,
	result domain.PaymentVerificationResult,
) error {
	sub, err := domain.NewSubscription(
		result.UserID(),
		result.Provider(),
		result.ProductID(),
		result.PurchaseTime(),
		result.ExpirationTime(),
	)
	if err != nil {
		return fmt.Errorf("creating subscription: %w", err)
	}
	if err := s.subscriptions.Upsert(ctx, sub); err != nil {
		return fmt.Errorf("upserting subscription: %w", err)
	}
	return nil
}

func (s *subscriptionService) IsActive(ctx context.Context, userID string) (bool, error) {
	sub, found, err := s.subscriptions.FindByUserID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("checking subscription: %w", err)
	}
	if !found {
		return false, nil
	}
	return sub.IsActive(time.Now().UTC()), nil
}

func (s *subscriptionService) GetSubscription(
	ctx context.Context,
	userID string,
) (domain.Subscription, bool, error) {
	return s.subscriptions.FindByUserID(ctx, userID)
}
