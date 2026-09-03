package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pano_chart/backend/domain"
)

func TestNewSubscription_ValidInput(t *testing.T) {
	start := time.Now().UTC()
	exp := start.Add(30 * 24 * time.Hour)

	sub, err := domain.NewSubscription("user1", "google_play", "premium_monthly", start, exp)
	require.NoError(t, err)
	assert.Equal(t, "user1", sub.UserID())
	assert.Equal(t, "google_play", sub.Provider())
	assert.Equal(t, "premium_monthly", sub.ProductID())
	assert.Equal(t, start, sub.StartTime())
	assert.Equal(t, exp, sub.ExpirationTime())
}

func TestNewSubscription_EmptyUserID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewSubscription("", "stripe", "prod1", now, now.Add(time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

func TestNewSubscription_EmptyProvider(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewSubscription("u1", "", "prod1", now, now.Add(time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestNewSubscription_EmptyProductID(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewSubscription("u1", "stripe", "", now, now.Add(time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "product_id")
}

func TestNewSubscription_ExpirationBeforeStart(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewSubscription("u1", "stripe", "prod1", now, now.Add(-time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expiration_time")
}

func TestSubscription_IsActive(t *testing.T) {
	start := time.Now().UTC()
	exp := start.Add(30 * 24 * time.Hour)

	sub, _ := domain.NewSubscription("u1", "stripe", "prod1", start, exp)
	assert.True(t, sub.IsActive(start.Add(time.Hour)))
	assert.False(t, sub.IsActive(exp.Add(time.Hour)))
}

func TestSubscription_IsActive_Expired(t *testing.T) {
	start := time.Now().UTC().Add(-60 * 24 * time.Hour)
	exp := start.Add(30 * 24 * time.Hour) // expired 30 days ago

	sub, _ := domain.NewSubscription("u1", "stripe", "prod1", start, exp)
	assert.False(t, sub.IsActive(time.Now().UTC()))
}
