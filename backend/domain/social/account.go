package social

// Account represents a social media account being tracked.
type Account struct {
	ID       string // unique identifier (platform:handle)
	Platform string // e.g. "twitter", "threads", "truth"
	Handle   string // e.g. "realDonaldTrump"

	LastSeenPostID    string // GUID of the most recent post seen
	LastSeenTimestamp int64  // unix timestamp of the most recent post seen (dedup fallback)
	LastPolledAt      int64  // unix timestamp of last successful poll
	LastUsedAt        int64  // unix timestamp of last user access (for cleanup)
}

// NewAccount creates a new Account with a deterministic ID.
func NewAccount(platform, handle string) Account {
	return Account{
		ID:       platform + ":" + handle,
		Platform: platform,
		Handle:   handle,
	}
}
