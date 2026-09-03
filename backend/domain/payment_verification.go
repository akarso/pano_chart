package domain

import (
	"fmt"
	"time"
)

// PaymentVerificationResult is the normalized output of a provider
// verification call.  All providers must map their response to this format.
type PaymentVerificationResult struct {
	valid                 bool
	provider              string
	externalTransactionID string
	productID             string
	userID                string
	purchaseTime          time.Time
	expirationTime        time.Time
}

// NewPaymentVerificationResult creates a validated result.
func NewPaymentVerificationResult(
	valid bool,
	provider string,
	externalTransactionID string,
	productID string,
	userID string,
	purchaseTime time.Time,
	expirationTime time.Time,
) (PaymentVerificationResult, error) {
	if provider == "" {
		return PaymentVerificationResult{}, fmt.Errorf("verification result provider cannot be empty")
	}
	if valid {
		if externalTransactionID == "" {
			return PaymentVerificationResult{}, fmt.Errorf("valid verification must have external_transaction_id")
		}
		if productID == "" {
			return PaymentVerificationResult{}, fmt.Errorf("valid verification must have product_id")
		}
		if userID == "" {
			return PaymentVerificationResult{}, fmt.Errorf("valid verification must have user_id")
		}
	}

	return PaymentVerificationResult{
		valid:                 valid,
		provider:              provider,
		externalTransactionID: externalTransactionID,
		productID:             productID,
		userID:                userID,
		purchaseTime:          purchaseTime,
		expirationTime:        expirationTime,
	}, nil
}

func (r PaymentVerificationResult) Valid() bool                   { return r.valid }
func (r PaymentVerificationResult) Provider() string              { return r.provider }
func (r PaymentVerificationResult) ExternalTransactionID() string { return r.externalTransactionID }
func (r PaymentVerificationResult) ProductID() string             { return r.productID }
func (r PaymentVerificationResult) UserID() string                { return r.userID }
func (r PaymentVerificationResult) PurchaseTime() time.Time       { return r.purchaseTime }
func (r PaymentVerificationResult) ExpirationTime() time.Time     { return r.expirationTime }

// NewPaymentVerificationResultUnsafe creates a result without validation (test only).
func NewPaymentVerificationResultUnsafe(
	valid bool,
	provider, txID, productID, userID string,
	purchaseTime, expirationTime time.Time,
) PaymentVerificationResult {
	return PaymentVerificationResult{
		valid:                 valid,
		provider:              provider,
		externalTransactionID: txID,
		productID:             productID,
		userID:                userID,
		purchaseTime:          purchaseTime,
		expirationTime:        expirationTime,
	}
}
