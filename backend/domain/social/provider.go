package social

// Provider fetches recent posts for a tracked social account.
//
// Implementations are platform-specific (RSS/Nitter, direct API, scraper, etc.)
// and are registered with the Poller at startup.
type Provider interface {
	// Platform returns the provider's platform identifier (e.g. "twitter").
	Platform() string

	// Fetch retrieves the most recent posts for the given account.
	Fetch(account Account) ([]Post, error)
}
