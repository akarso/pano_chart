package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/domain"
	"pano_chart/backend/infrastructure/payment"
)

func newTestRepo(t *testing.T) *payment.SQLiteRepository {
	t.Helper()
	repo, err := payment.NewSQLiteRepository(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestSQLiteRepository_SaveAndFind(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(30 * 24 * time.Hour)

	p, err := domain.NewPurchase("user1", "stripe", "tx_1", "premium", now, exp, true)
	require.NoError(t, err)

	id, err := repo.Save(ctx, p)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	found, ok, err := repo.FindByTransactionID(ctx, "stripe", "tx_1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "user1", found.UserID())
	assert.Equal(t, "stripe", found.Provider())
	assert.Equal(t, "tx_1", found.ExternalTransactionID())
	assert.True(t, found.Verified())
}

func TestSQLiteRepository_FindByTransactionID_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, ok, err := repo.FindByTransactionID(ctx, "stripe", "nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSQLiteRepository_DuplicateTransaction_Rejected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(30 * 24 * time.Hour)

	p, _ := domain.NewPurchase("user1", "stripe", "tx_dup", "premium", now, exp, true)

	_, err := repo.Save(ctx, p)
	require.NoError(t, err)

	// Second save with same provider + tx ID should fail (UNIQUE constraint).
	_, err = repo.Save(ctx, p)
	assert.Error(t, err)
}

func TestSQLiteRepository_UpsertAndFindSubscription(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(30 * 24 * time.Hour)

	sub, err := domain.NewSubscription("user1", "stripe", "premium", now, exp)
	require.NoError(t, err)

	err = repo.Upsert(ctx, sub)
	require.NoError(t, err)

	found, ok, err := repo.FindByUserID(ctx, "user1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "user1", found.UserID())
	assert.Equal(t, "stripe", found.Provider())
	assert.Equal(t, "premium", found.ProductID())
}

func TestSQLiteRepository_Upsert_UpdatesExisting(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	sub1, _ := domain.NewSubscription("user1", "stripe", "basic", now, now.Add(7*24*time.Hour))
	err := repo.Upsert(ctx, sub1)
	require.NoError(t, err)

	// Upsert with new product.
	sub2, _ := domain.NewSubscription("user1", "stripe", "premium", now, now.Add(30*24*time.Hour))
	err = repo.Upsert(ctx, sub2)
	require.NoError(t, err)

	found, ok, err := repo.FindByUserID(ctx, "user1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "premium", found.ProductID())
}

func TestSQLiteRepository_FindByUserID_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, ok, err := repo.FindByUserID(ctx, "nobody")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSQLiteRepository_ApplyVerifiedPurchase_ChainedRebinds(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(30 * 24 * time.Hour)

	p, err := domain.NewPurchase("user1", "google_play", "tx_chain", "premium", now, exp, true)
	require.NoError(t, err)
	_, err = repo.Save(ctx, p)
	require.NoError(t, err)

	sub1, err := domain.NewSubscription("user1", "google_play", "premium", now, exp)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, sub1))

	moveTo := func(to string) {
		t.Helper()
		result, err := domain.NewPaymentVerificationResult(
			true, "google_play", "tx_chain", "premium", to, now, exp,
		)
		require.NoError(t, err)
		require.NoError(t, repo.ApplyVerifiedPurchase(ctx, result))
	}

	moveTo("user2")
	moveTo("user3")

	found, ok, err := repo.FindByTransactionID(ctx, "google_play", "tx_chain")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "user3", found.UserID())

	for _, uid := range []string{"user1", "user2"} {
		sub, ok, err := repo.FindByUserID(ctx, uid)
		require.NoError(t, err)
		require.True(t, ok)
		assert.False(t, sub.IsActive(time.Now().UTC()), "%s should be expired", uid)
	}

	sub3, ok, err := repo.FindByUserID(ctx, "user3")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, sub3.IsActive(time.Now().UTC()))
}

// Concurrent rebinds must leave only the final purchase owner entitled.
// Before the owner-checked UPDATE, two devices that both read owner A could
// each grant themselves an active subscription while only expiring A.
func TestSQLiteRepository_ApplyVerifiedPurchase_ConcurrentRebindsExpireLosers(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(30 * 24 * time.Hour)

	p, err := domain.NewPurchase("user-a", "google_play", "tx_race", "premium", now, exp, true)
	require.NoError(t, err)
	_, err = repo.Save(ctx, p)
	require.NoError(t, err)
	subA, err := domain.NewSubscription("user-a", "google_play", "premium", now, exp)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, subA))

	users := []string{"user-b", "user-c", "user-d", "user-e"}
	errCh := make(chan error, len(users))
	for _, u := range users {
		go func(uid string) {
			result, err := domain.NewPaymentVerificationResult(
				true, "google_play", "tx_race", "premium", uid, now, exp,
			)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- repo.ApplyVerifiedPurchase(ctx, result)
		}(u)
	}
	for range users {
		require.NoError(t, <-errCh)
	}

	found, ok, err := repo.FindByTransactionID(ctx, "google_play", "tx_race")
	require.NoError(t, err)
	require.True(t, ok)
	owner := found.UserID()

	activeCount := 0
	for _, uid := range append([]string{"user-a"}, users...) {
		sub, ok, err := repo.FindByUserID(ctx, uid)
		require.NoError(t, err)
		if !ok {
			continue
		}
		if sub.IsActive(time.Now().UTC()) {
			activeCount++
			assert.Equal(t, owner, uid, "only the purchase owner may remain active")
		}
	}
	assert.Equal(t, 1, activeCount, "exactly one user must remain entitled")
}
