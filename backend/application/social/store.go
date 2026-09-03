package social

import (
	"errors"

	domain "pano_chart/backend/domain/social"
)

// ErrDeviceOwnedByAnotherUser is returned by DeviceTokenStore.Register when
// deviceID is already registered to a different user — Register no longer
// silently reassigns ownership.
var ErrDeviceOwnedByAnotherUser = errors.New("device already registered to a different user")

// AccountStore persists tracked social accounts.
type AccountStore interface {
	// Upsert creates or updates an account.
	Upsert(account domain.Account) error

	// Get returns a single account by ID, or nil if not found.
	Get(accountID string) (*domain.Account, error)

	// GetAllActive returns every account that has at least one subscriber.
	GetAllActive() ([]domain.Account, error)

	// MarkUsed bumps the LastUsedAt timestamp for the given account.
	MarkUsed(accountID string) error

	// CleanupUnused removes accounts whose LastUsedAt is older than
	// thresholdUnix. Returns the number of accounts removed.
	CleanupUnused(thresholdUnix int64) (int, error)
}

// SubscriptionStore persists user → account subscriptions.
type SubscriptionStore interface {
	// Subscribe adds a subscription (idempotent).
	Subscribe(userID, accountID string) error

	// Unsubscribe removes a subscription.
	Unsubscribe(userID, accountID string) error

	// AccountsForUser returns the account IDs a user is subscribed to.
	AccountsForUser(userID string) ([]string, error)

	// HasSubscribers returns true if at least one user subscribes to accountID.
	HasSubscribers(accountID string) (bool, error)

	// UsersForAccount returns all user IDs subscribed to the given account.
	UsersForAccount(accountID string) ([]string, error)

	// SetFilterConfig stores per-user per-account filter settings.
	SetFilterConfig(userID, accountID string, config FeedFilter) error

	// FilterConfigForAccount returns the filter config for each subscriber
	// of the given account. The map key is the user ID.
	FilterConfigForAccount(accountID string) (map[string]FeedFilter, error)
}

// DeviceTokenStore persists FCM device tokens keyed by user.
type DeviceTokenStore interface {
	// Register stores or updates a device's FCM token for a user. If
	// deviceID already belongs to a different user, returns
	// ErrDeviceOwnedByAnotherUser instead of silently reassigning it.
	Register(userID, deviceID, fcmToken, platform string) error

	// Unregister removes a device token, scoped to the owning user so one
	// user cannot remove another user's registration.
	Unregister(userID, deviceID string) error

	// TokensForUsers returns all distinct FCM tokens for the given user IDs.
	TokensForUsers(userIDs []string) ([]string, error)

	// AllTokens returns every registered FCM token (for broadcast notifications).
	AllTokens() ([]string, error)
}
