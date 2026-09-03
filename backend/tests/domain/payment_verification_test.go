package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/domain"
)

func TestNewPaymentVerificationResult_Valid(t *testing.T) {
	now := time.Now().UTC()
	exp := now.Add(30 * 24 * time.Hour)

	r, err := domain.NewPaymentVerificationResult(true, "stripe", "tx1", "prod1", "user1", now, exp)
	require.NoError(t, err)
	assert.True(t, r.Valid())
	assert.Equal(t, "stripe", r.Provider())
	assert.Equal(t, "tx1", r.ExternalTransactionID())
	assert.Equal(t, "prod1", r.ProductID())
	assert.Equal(t, "user1", r.UserID())
}

func TestNewPaymentVerificationResult_Invalid(t *testing.T) {
	now := time.Now().UTC()
	// Invalid result is allowed to have empty fields except provider.
	r, err := domain.NewPaymentVerificationResult(false, "stripe", "", "", "", now, now)
	require.NoError(t, err)
	assert.False(t, r.Valid())
}

func TestNewPaymentVerificationResult_EmptyProvider(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPaymentVerificationResult(true, "", "tx1", "prod1", "user1", now, now)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestNewPaymentVerificationResult_ValidMissingTxID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPaymentVerificationResult(true, "stripe", "", "prod1", "user1", now, now)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "external_transaction_id")
}

func TestNewPaymentVerificationResult_ValidMissingProductID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPaymentVerificationResult(true, "stripe", "tx1", "", "user1", now, now)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "product_id")
}

func TestNewPaymentVerificationResult_ValidMissingUserID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPaymentVerificationResult(true, "stripe", "tx1", "prod1", "", now, now)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}
