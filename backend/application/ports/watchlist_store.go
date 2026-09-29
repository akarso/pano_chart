package ports

import (
	"context"
	"errors"
)

// WatchlistMaxSymbols is the maximum number of symbols one user may keep on
// their watchlist (ROADMAP PR-101).
const WatchlistMaxSymbols = 50

// WatchlistMaxSymbolLength bounds one symbol string's length — generous
// headroom over any real trading pair (e.g. "1000SHIBUSDT" is 12 chars),
// just enough to stop an arbitrarily long client-supplied string from
// bloating the watchlist table with no size guard beyond symbol count.
const WatchlistMaxSymbolLength = 32

// ErrWatchlistTooLarge is returned by Replace when symbols would exceed
// WatchlistMaxSymbols. Nothing is written when this is returned.
var ErrWatchlistTooLarge = errors.New("watchlist: too many symbols")

// WatchlistStore persists each user's watchlisted symbols.
type WatchlistStore interface {
	// Get returns userID's watchlisted symbols, oldest-added first.
	Get(ctx context.Context, userID string) ([]string, error)

	// Replace overwrites userID's entire watchlist with symbols — the app
	// resyncs its full local (offline-cached) state on every star toggle,
	// so this is a replace, not a merge; a symbol already on the list keeps
	// its original added-at ordering, only newly-appearing symbols are
	// timestamped now. Returns ErrWatchlistTooLarge, writing nothing, if
	// len(symbols) > WatchlistMaxSymbols.
	Replace(ctx context.Context, userID string, symbols []string) error

	// Remove deletes the given symbols from userID's watchlist. Symbols
	// not on the watchlist are silently ignored.
	Remove(ctx context.Context, userID string, symbols []string) error
}
