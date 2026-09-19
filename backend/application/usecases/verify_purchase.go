package usecases

import (
	"context"
	"fmt"

	"pano_chart/backend/application/ports"
)

// VerifyPurchase orchestrates payment verification.
//
// Flow: look up provider from registry → verify token → activate subscription.
type VerifyPurchase interface {
	Execute(ctx context.Context, input VerifyPurchaseInput) error
}

// VerifyPurchaseInput carries the data needed to verify a purchase.
type VerifyPurchaseInput struct {
	Provider      string
	PurchaseToken string
	UserID        string
}

type verifyPurchase struct {
	registry     *PaymentProviderRegistry
	subscription SubscriptionService
}

// NewVerifyPurchase constructs the use case.
func NewVerifyPurchase(
	registry *PaymentProviderRegistry,
	subscription SubscriptionService,
) VerifyPurchase {
	return &verifyPurchase{
		registry:     registry,
		subscription: subscription,
	}
}

func (uc *verifyPurchase) Execute(ctx context.Context, input VerifyPurchaseInput) error {
	if input.Provider == "" {
		return fmt.Errorf("%w: provider is required", ports.ErrInvalidPurchaseToken)
	}
	if input.PurchaseToken == "" {
		return fmt.Errorf("%w: purchase_token is required", ports.ErrInvalidPurchaseToken)
	}
	if input.UserID == "" {
		return fmt.Errorf("%w: user_id is required", ports.ErrInvalidPurchaseToken)
	}

	provider, err := uc.registry.Get(input.Provider)
	if err != nil {
		return fmt.Errorf("provider lookup: %w", err)
	}

	result, err := provider.VerifyPurchase(ctx, input.PurchaseToken, input.UserID)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	if !result.Valid() {
		return fmt.Errorf("%w: purchase verification returned invalid result", ports.ErrInvalidPurchaseToken)
	}

	if err := uc.subscription.ActivateSubscription(ctx, result); err != nil {
		return fmt.Errorf("activating subscription: %w", err)
	}

	return nil
}
