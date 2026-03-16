package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

func makeVerifyPurchase(
	provider *fakePaymentProvider,
) (usecases.VerifyPurchase, *fakePurchaseRepository, *fakeSubscriptionRepository) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()

	registry := usecases.NewPaymentProviderRegistry()
	registry.Register(provider)

	subSvc := usecases.NewSubscriptionService(purchases, subs)
	uc := usecases.NewVerifyPurchase(registry, subSvc)

	return uc, purchases, subs
}

func TestVerifyPurchase_HappyPath(t *testing.T) {
	now := time.Now().UTC()
	result, _ := domain.NewPaymentVerificationResult(
		true, "test_pay", "tx1", "premium", "user1",
		now, now.Add(30*24*time.Hour),
	)

	provider := &fakePaymentProvider{name: "test_pay", result: result}
	uc, purchases, subs := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "token123",
		UserID:        "user1",
	})

	require.NoError(t, err)
	assert.Len(t, purchases.saved, 1)
	assert.Contains(t, subs.subs, "user1")
}

func TestVerifyPurchase_EmptyProvider(t *testing.T) {
	provider := &fakePaymentProvider{name: "test_pay"}
	uc, _, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "",
		PurchaseToken: "token",
		UserID:        "user1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestVerifyPurchase_EmptyToken(t *testing.T) {
	provider := &fakePaymentProvider{name: "test_pay"}
	uc, _, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "",
		UserID:        "user1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "purchase_token")
}

func TestVerifyPurchase_EmptyUserID(t *testing.T) {
	provider := &fakePaymentProvider{name: "test_pay"}
	uc, _, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "token",
		UserID:        "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

func TestVerifyPurchase_UnknownProvider(t *testing.T) {
	provider := &fakePaymentProvider{name: "test_pay"}
	uc, _, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "unknown_provider",
		PurchaseToken: "token",
		UserID:        "user1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestVerifyPurchase_ProviderError(t *testing.T) {
	provider := &fakePaymentProvider{
		name: "test_pay",
		err:  errors.New("network timeout"),
	}
	uc, _, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "token",
		UserID:        "user1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestVerifyPurchase_InvalidVerificationResult(t *testing.T) {
	now := time.Now().UTC()
	invalid, _ := domain.NewPaymentVerificationResult(
		false, "test_pay", "", "", "", now, now,
	)

	provider := &fakePaymentProvider{name: "test_pay", result: invalid}
	uc, purchases, _ := makeVerifyPurchase(provider)

	err := uc.Execute(context.Background(), usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "token",
		UserID:        "user1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid result")
	assert.Empty(t, purchases.saved)
}

func TestVerifyPurchase_DuplicateTransaction(t *testing.T) {
	now := time.Now().UTC()
	result, _ := domain.NewPaymentVerificationResult(
		true, "test_pay", "tx_dup", "premium", "user1",
		now, now.Add(30*24*time.Hour),
	)

	provider := &fakePaymentProvider{name: "test_pay", result: result}
	uc, _, _ := makeVerifyPurchase(provider)

	input := usecases.VerifyPurchaseInput{
		Provider:      "test_pay",
		PurchaseToken: "token",
		UserID:        "user1",
	}

	err := uc.Execute(context.Background(), input)
	require.NoError(t, err)

	// Second attempt with same transaction ID should fail.
	err = uc.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate transaction")
}
