package ports

import "errors"

// Sentinel errors for payment-provider failures. HTTP handlers map these to
// stable client codes via errors.Is — do not rely on error-string matching.
var (
	// ErrProviderRateLimited means the upstream store throttled the call
	// (e.g. Google Play HTTP 429). Clients should retry later.
	ErrProviderRateLimited = errors.New("payment provider rate limited")

	// ErrProviderUnavailable means a transient upstream outage or network
	// failure (5xx, timeout, connection errors). Clients should retry; this
	// must not be reported as an invalid purchase token.
	ErrProviderUnavailable = errors.New("payment provider unavailable")

	// ErrInvalidPurchaseToken means the provider explicitly rejected the
	// token (or the token/result is known-invalid). Clients must not retry
	// with the same token without user action.
	ErrInvalidPurchaseToken = errors.New("invalid purchase token")

	// ErrUnsupportedProvider means the request named a payment provider
	// that is not registered. This is a client validation error (4xx), not
	// a server outage.
	ErrUnsupportedProvider = errors.New("unsupported payment provider")
)
