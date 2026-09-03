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
	// user's subscription.  Returns an error if the verification result
	// is not valid.
	ActivateSubscription(ctx context.Context, result domain.PaymentVerificationResult) error

	// IsActive checks whether the user has an active (non-expired)
	// subscription.
	IsActive(ctx context.Context, userID string) (bool, error)

	// GetSubscription returns the subscription for the user, if any.
	GetSubscription(ctx context.Context, userID string) (domain.Subscription, bool, error)
}

type subscriptionService struct {
	purchases     ports.PurchaseRepository
	subscriptions ports.SubscriptionRepository
}

// NewSubscriptionService constructs a SubscriptionService backed by the
// given repositories.
func NewSubscriptionService(
	purchases ports.PurchaseRepository,
	subscriptions ports.SubscriptionRepository,
) SubscriptionService {
	return &subscriptionService{
		purchases:     purchases,
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

	// Guard against replay: reject if transaction already recorded.
	_, exists, err := s.purchases.FindByTransactionID(
		ctx, result.Provider(), result.ExternalTransactionID(),
	)
	if err != nil {
		return fmt.Errorf("checking duplicate transaction: %w", err)
	}
	if exists {
		return fmt.Errorf("duplicate transaction %s from provider %s",
			result.ExternalTransactionID(), result.Provider())
	}

	// Record the purchase.
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

	// Upsert subscription.
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
