package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

func validVerificationResult() domain.PaymentVerificationResult {
	now := time.Now().UTC()
	r, _ := domain.NewPaymentVerificationResult(
		true, "stripe", "tx_abc", "premium_monthly", "user1",
		now, now.Add(30*24*time.Hour),
	)
	return r
}

func TestSubscriptionService_ActivateSubscription_HappyPath(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())

	require.NoError(t, err)
	assert.Len(t, purchases.saved, 1)
	assert.Contains(t, subs.subs, "user1")
}

func TestSubscriptionService_ActivateSubscription_InvalidResult(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	now := time.Now().UTC()
	invalid, _ := domain.NewPaymentVerificationResult(false, "stripe", "", "", "", now, now)

	err := svc.ActivateSubscription(context.Background(), invalid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
	assert.Empty(t, purchases.saved)
}

func TestSubscriptionService_ActivateSubscription_DuplicateTransaction(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	result := validVerificationResult()

	err := svc.ActivateSubscription(context.Background(), result)
	require.NoError(t, err)

	// Second attempt with same transaction should fail.
	err = svc.ActivateSubscription(context.Background(), result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate transaction")
}

func TestSubscriptionService_IsActive_HappyPath(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())
	require.NoError(t, err)

	active, err := svc.IsActive(context.Background(), "user1")
	require.NoError(t, err)
	assert.True(t, active)
}

func TestSubscriptionService_IsActive_NoSubscription(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	active, err := svc.IsActive(context.Background(), "unknown_user")
	require.NoError(t, err)
	assert.False(t, active)
}

func TestSubscriptionService_IsActive_Expired(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	now := time.Now().UTC()
	expiredSub := domain.NewSubscriptionUnsafe(
		"user_expired", "stripe", "prod1",
		now.Add(-60*24*time.Hour), now.Add(-30*24*time.Hour), now,
	)
	subs.subs["user_expired"] = expiredSub

	active, err := svc.IsActive(context.Background(), "user_expired")
	require.NoError(t, err)
	assert.False(t, active)
}

func TestSubscriptionService_GetSubscription_Found(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())
	require.NoError(t, err)

	sub, found, err := svc.GetSubscription(context.Background(), "user1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "user1", sub.UserID())
}

func TestSubscriptionService_GetSubscription_NotFound(t *testing.T) {
	purchases := newFakePurchaseRepository()
	subs := newFakeSubscriptionRepository()
	svc := usecases.NewSubscriptionService(purchases, subs)

	_, found, err := svc.GetSubscription(context.Background(), "nobody")
	require.NoError(t, err)
	assert.False(t, found)
}
