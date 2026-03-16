package domain

import (
	"fmt"
	"time"
)

// Purchase records a single payment attempt from any provider.
type Purchase struct {
	id                    int64
	userID                string
	provider              string
	externalTransactionID string
	productID             string
	purchaseTime          time.Time
	expirationTime        time.Time
	verified              bool
	createdAt             time.Time
}

// NewPurchase creates a validated Purchase.
func NewPurchase(
	userID string,
	provider string,
	externalTransactionID string,
	productID string,
	purchaseTime time.Time,
	expirationTime time.Time,
	verified bool,
) (Purchase, error) {
	if userID == "" {
		return Purchase{}, fmt.Errorf("purchase user_id cannot be empty")
	}
	if provider == "" {
		return Purchase{}, fmt.Errorf("purchase provider cannot be empty")
	}
	if externalTransactionID == "" {
		return Purchase{}, fmt.Errorf("purchase external_transaction_id cannot be empty")
	}
	if productID == "" {
		return Purchase{}, fmt.Errorf("purchase product_id cannot be empty")
	}
	if purchaseTime.Location() != time.UTC || expirationTime.Location() != time.UTC {
		return Purchase{}, fmt.Errorf("purchase times must be UTC")
	}
	if expirationTime.Before(purchaseTime) {
		return Purchase{}, fmt.Errorf("expiration_time cannot be before purchase_time")
	}

	return Purchase{
		userID:                userID,
		provider:              provider,
		externalTransactionID: externalTransactionID,
		productID:             productID,
		purchaseTime:          purchaseTime,
		expirationTime:        expirationTime,
		verified:              verified,
		createdAt:             time.Now().UTC(),
	}, nil
}

func (p Purchase) ID() int64                     { return p.id }
func (p Purchase) UserID() string                { return p.userID }
func (p Purchase) Provider() string              { return p.provider }
func (p Purchase) ExternalTransactionID() string { return p.externalTransactionID }
func (p Purchase) ProductID() string             { return p.productID }
func (p Purchase) PurchaseTime() time.Time       { return p.purchaseTime }
func (p Purchase) ExpirationTime() time.Time     { return p.expirationTime }
func (p Purchase) Verified() bool                { return p.verified }
func (p Purchase) CreatedAt() time.Time          { return p.createdAt }

// NewPurchaseUnsafe creates a Purchase without validation (test only).
func NewPurchaseUnsafe(
	id int64,
	userID, provider, txID, productID string,
	purchaseTime, expirationTime, createdAt time.Time,
	verified bool,
) Purchase {
	return Purchase{
		id:                    id,
		userID:                userID,
		provider:              provider,
		externalTransactionID: txID,
		productID:             productID,
		purchaseTime:          purchaseTime,
		expirationTime:        expirationTime,
		verified:              verified,
		createdAt:             createdAt,
	}
}
