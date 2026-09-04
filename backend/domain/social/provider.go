package social

import "context"

// Provider fetches recent posts for a tracked social account.
//
// Implementations are platform-specific (RSS/Nitter, direct API, scraper, etc.)
// and are registered with the Poller at startup.
type Provider interface {
	// Platform returns the provider's platform identifier (e.g. "twitter").
	Platform() string

	// Fetch retrieves the most recent posts for the given account. Must
	// honor ctx cancellation — see PR-076 CR follow-up: Watcher.Run's
	// context is cancelled during graceful shutdown, and a Fetch that
	// can't be aborted keeps pollNext() from returning promptly, which
	// can let shutdown's bounded wait for background workers expire while
	// a fetch (and the account-store write that follows a successful one)
	// is still in flight against a store that's about to be closed.
	Fetch(ctx context.Context, account Account) ([]Post, error)
}
