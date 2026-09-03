package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/domain"
)

func TestNewPurchase_ValidInput(t *testing.T) {
	now := time.Now().UTC()
	exp := now.Add(30 * 24 * time.Hour)

	p, err := domain.NewPurchase("user1", "google_play", "tx123", "premium_monthly", now, exp, true)
	require.NoError(t, err)
	assert.Equal(t, "user1", p.UserID())
	assert.Equal(t, "google_play", p.Provider())
	assert.Equal(t, "tx123", p.ExternalTransactionID())
	assert.Equal(t, "premium_monthly", p.ProductID())
	assert.True(t, p.Verified())
	assert.Equal(t, now, p.PurchaseTime())
	assert.Equal(t, exp, p.ExpirationTime())
}

func TestNewPurchase_EmptyUserID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPurchase("", "stripe", "tx1", "prod1", now, now.Add(time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

func TestNewPurchase_EmptyProvider(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPurchase("u1", "", "tx1", "prod1", now, now.Add(time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestNewPurchase_EmptyTransactionID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPurchase("u1", "stripe", "", "prod1", now, now.Add(time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "external_transaction_id")
}

func TestNewPurchase_EmptyProductID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPurchase("u1", "stripe", "tx1", "", now, now.Add(time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "product_id")
}

func TestNewPurchase_ExpirationBeforePurchase(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPurchase("u1", "stripe", "tx1", "prod1", now, now.Add(-time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expiration_time")
}

func TestNewPurchase_NonUTCTime(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	now := time.Now().In(loc)
	_, err := domain.NewPurchase("u1", "stripe", "tx1", "prod1", now, now.Add(time.Hour), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UTC")
}

func TestPurchaseUnsafe(t *testing.T) {
	now := time.Now().UTC()
	p := domain.NewPurchaseUnsafe(42, "u1", "stripe", "tx1", "prod1", now, now.Add(time.Hour), now, true)
	assert.Equal(t, int64(42), p.ID())
	assert.Equal(t, "u1", p.UserID())
}
