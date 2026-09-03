package domain

import (
	"fmt"
	"time"
)

// Subscription tracks the active subscription state for a user.
type Subscription struct {
	userID         string
	provider       string
	productID      string
	startTime      time.Time
	expirationTime time.Time
	updatedAt      time.Time
}

// NewSubscription creates a validated Subscription.
func NewSubscription(
	userID string,
	provider string,
	productID string,
	startTime time.Time,
	expirationTime time.Time,
) (Subscription, error) {
	if userID == "" {
		return Subscription{}, fmt.Errorf("subscription user_id cannot be empty")
	}
	if provider == "" {
		return Subscription{}, fmt.Errorf("subscription provider cannot be empty")
	}
	if productID == "" {
		return Subscription{}, fmt.Errorf("subscription product_id cannot be empty")
	}
	if startTime.Location() != time.UTC || expirationTime.Location() != time.UTC {
		return Subscription{}, fmt.Errorf("subscription times must be UTC")
	}
	if expirationTime.Before(startTime) {
		return Subscription{}, fmt.Errorf("expiration_time cannot be before start_time")
	}

	return Subscription{
		userID:         userID,
		provider:       provider,
		productID:      productID,
		startTime:      startTime,
		expirationTime: expirationTime,
		updatedAt:      time.Now().UTC(),
	}, nil
}

func (s Subscription) UserID() string            { return s.userID }
func (s Subscription) Provider() string          { return s.provider }
func (s Subscription) ProductID() string         { return s.productID }
func (s Subscription) StartTime() time.Time      { return s.startTime }
func (s Subscription) ExpirationTime() time.Time { return s.expirationTime }
func (s Subscription) UpdatedAt() time.Time      { return s.updatedAt }

// IsActive returns true if the subscription has not expired relative to now.
func (s Subscription) IsActive(now time.Time) bool {
	return now.Before(s.expirationTime)
}

// NewSubscriptionUnsafe creates a Subscription without validation (test only).
func NewSubscriptionUnsafe(
	userID, provider, productID string,
	startTime, expirationTime, updatedAt time.Time,
) Subscription {
	return Subscription{
		userID:         userID,
		provider:       provider,
		productID:      productID,
		startTime:      startTime,
		expirationTime: expirationTime,
		updatedAt:      updatedAt,
	}
}
