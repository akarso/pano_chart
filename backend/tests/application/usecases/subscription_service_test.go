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
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())

	require.NoError(t, err)
	assert.Len(t, purchases.saved, 1)
	assert.Contains(t, subs.subs, "user1")
}

func TestSubscriptionService_ActivateSubscription_InvalidResult(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	now := time.Now().UTC()
	invalid, _ := domain.NewPaymentVerificationResult(false, "stripe", "", "", "", now, now)

	err := svc.ActivateSubscription(context.Background(), invalid)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
	assert.Empty(t, purchases.saved)
}

func TestSubscriptionService_ActivateSubscription_DuplicateTransaction_IsIdempotent(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	first := validVerificationResult()
	err := svc.ActivateSubscription(context.Background(), first)
	require.NoError(t, err)

	// Restore / retry of the same Google order must succeed and refresh
	// expiry — rejecting it is what the app reports as "we could not
	// verify your purchase" after Play already charged the user.
	later := time.Now().UTC().Add(60 * 24 * time.Hour)
	replay, err := domain.NewPaymentVerificationResult(
		true, first.Provider(), first.ExternalTransactionID(), first.ProductID(), first.UserID(),
		first.PurchaseTime(), later,
	)
	require.NoError(t, err)

	err = svc.ActivateSubscription(context.Background(), replay)
	require.NoError(t, err)
	assert.Len(t, purchases.saved, 1, "must not insert a second purchase row")

	sub, found, err := svc.GetSubscription(context.Background(), "user1")
	require.NoError(t, err)
	require.True(t, found)
	assert.WithinDuration(t, later, sub.ExpirationTime(), time.Second)

	p, ok, err := purchases.FindByTransactionID(context.Background(), first.Provider(), first.ExternalTransactionID())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "user1", p.UserID())
	assert.WithinDuration(t, later, p.ExpirationTime(), time.Second)
}

func TestSubscriptionService_ActivateSubscription_RebindToNewDeviceIdentity(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	original := validVerificationResult()
	require.NoError(t, svc.ActivateSubscription(context.Background(), original))

	// Reinstall / 401 reclaim mints a new device user ID. The Play token
	// is still valid for the same Google account — access must follow
	// the token, not stay glued to the first device identity.
	moved, err := domain.NewPaymentVerificationResult(
		true, original.Provider(), original.ExternalTransactionID(), original.ProductID(), "user-reinstall",
		original.PurchaseTime(), original.ExpirationTime(),
	)
	require.NoError(t, err)

	require.NoError(t, svc.ActivateSubscription(context.Background(), moved))

	oldActive, err := svc.IsActive(context.Background(), "user1")
	require.NoError(t, err)
	assert.False(t, oldActive)

	newActive, err := svc.IsActive(context.Background(), "user-reinstall")
	require.NoError(t, err)
	assert.True(t, newActive)

	p, ok, err := purchases.FindByTransactionID(context.Background(), original.Provider(), original.ExternalTransactionID())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "user-reinstall", p.UserID(), "purchase ownership must follow the rebind")
}

func TestSubscriptionService_ActivateSubscription_ChainedRebinds_OnlyLatestActive(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	original := validVerificationResult()
	require.NoError(t, svc.ActivateSubscription(context.Background(), original))

	moveTo := func(userID string) {
		t.Helper()
		moved, err := domain.NewPaymentVerificationResult(
			true, original.Provider(), original.ExternalTransactionID(), original.ProductID(), userID,
			original.PurchaseTime(), original.ExpirationTime(),
		)
		require.NoError(t, err)
		require.NoError(t, svc.ActivateSubscription(context.Background(), moved))
	}

	moveTo("user2")
	moveTo("user3")

	for _, uid := range []string{"user1", "user2"} {
		active, err := svc.IsActive(context.Background(), uid)
		require.NoError(t, err)
		assert.False(t, active, "%s must not remain entitled after later rebinds", uid)
	}

	active3, err := svc.IsActive(context.Background(), "user3")
	require.NoError(t, err)
	assert.True(t, active3)

	p, ok, err := purchases.FindByTransactionID(context.Background(), original.Provider(), original.ExternalTransactionID())
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "user3", p.UserID())
}

func TestSubscriptionService_IsActive_HappyPath(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())
	require.NoError(t, err)

	active, err := svc.IsActive(context.Background(), "user1")
	require.NoError(t, err)
	assert.True(t, active)
}

func TestSubscriptionService_IsActive_NoSubscription(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	active, err := svc.IsActive(context.Background(), "unknown_user")
	require.NoError(t, err)
	assert.False(t, active)
}

func TestSubscriptionService_IsActive_Expired(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
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
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	err := svc.ActivateSubscription(context.Background(), validVerificationResult())
	require.NoError(t, err)

	sub, found, err := svc.GetSubscription(context.Background(), "user1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "user1", sub.UserID())
}

func TestSubscriptionService_GetSubscription_NotFound(t *testing.T) {
	purchases, subs := newLinkedPaymentFakes()
	svc := usecases.NewSubscriptionService(purchases, subs)

	_, found, err := svc.GetSubscription(context.Background(), "nobody")
	require.NoError(t, err)
	assert.False(t, found)
}
