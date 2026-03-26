package notifications

import "context"

// NotificationConfig holds per-user notification preferences and thresholds.
type NotificationConfig struct {
	UserID     string
	Social     bool
	Macro      bool
	News       bool
	Uptrend    bool
	Downtrend  bool
	Sideways   bool
	SetupOfDay bool

	UptrendMinDominance   float64
	DowntrendMinDominance float64
	SidewaysMinDominance  float64
	SetupMinScore         float64

	UptrendTimeframe   string // e.g. "1h", "15m"
	DowntrendTimeframe string
	SidewaysTimeframe  string
	SetupTimeframe     string
}

// DefaultNotificationConfig returns sane defaults for a new user.
func DefaultNotificationConfig(userID string) NotificationConfig {
	return NotificationConfig{
		UserID:                userID,
		Social:                true,
		Macro:                 true,
		News:                  true,
		Uptrend:               true,
		Downtrend:             true,
		Sideways:              true,
		SetupOfDay:            true,
		UptrendMinDominance:   0.75,
		DowntrendMinDominance: 0.75,
		SidewaysMinDominance:  0.75,
		SetupMinScore:         0.75,
		UptrendTimeframe:      "1h",
		DowntrendTimeframe:    "1h",
		SidewaysTimeframe:     "1h",
		SetupTimeframe:        "1h",
	}
}

// NotificationConfigStore persists per-user notification configuration.
type NotificationConfigStore interface {
	// Get returns the config for a user. If none exists, returns defaults.
	Get(userID string) (NotificationConfig, error)
	// Save creates or updates the config for a user.
	Save(cfg NotificationConfig) error
	// All returns configs for all users that have registered one.
	All() ([]NotificationConfig, error)
}

// SubscriptionChecker reports whether a user has an active subscription.
// The notifications package uses this to gate pro-only notification types.
type SubscriptionChecker interface {
	IsActive(ctx context.Context, userID string) (bool, error)
}
