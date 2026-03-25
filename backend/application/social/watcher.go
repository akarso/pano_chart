package social

import (
	"context"
	"log"
	"time"

	domain "pano_chart/backend/domain/social"
)

// Watcher runs the background polling loop. It rotates through active
// accounts at a configurable interval (default 1 req/sec), fetches via
// the registered provider, deduplicates, caches, and dispatches new posts.
type Watcher struct {
	provider   domain.Provider
	cache      *PostCache
	accounts   AccountStore
	subs       SubscriptionStore
	scheduler  *Scheduler
	dispatcher *Dispatcher

	pollInterval    time.Duration
	refreshInterval time.Duration
	cleanupAge      time.Duration
}

// WatcherConfig holds tuning knobs for the background loop.
type WatcherConfig struct {
	PollInterval    time.Duration // how often to poll the next account (default: 1s)
	RefreshInterval time.Duration // how often to reload the account list (default: 30s)
	CleanupAge      time.Duration // remove accounts unused for this long (default: 24h)
}

// DefaultWatcherConfig returns the MVP defaults from PR-057.
func DefaultWatcherConfig() WatcherConfig {
	return WatcherConfig{
		PollInterval:    1 * time.Second,
		RefreshInterval: 30 * time.Second,
		CleanupAge:      24 * time.Hour,
	}
}

// NewWatcher creates a watcher with the given dependencies.
func NewWatcher(
	provider domain.Provider,
	cache *PostCache,
	accounts AccountStore,
	subs SubscriptionStore,
	dispatcher *Dispatcher,
	cfg WatcherConfig,
) *Watcher {
	return &Watcher{
		provider:        provider,
		cache:           cache,
		accounts:        accounts,
		subs:            subs,
		scheduler:       NewScheduler(),
		dispatcher:      dispatcher,
		pollInterval:    cfg.PollInterval,
		refreshInterval: cfg.RefreshInterval,
		cleanupAge:      cfg.CleanupAge,
	}
}

// Run starts the watcher loop. Blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.refreshAccounts()

	pollTicker := time.NewTicker(w.pollInterval)
	refreshTicker := time.NewTicker(w.refreshInterval)
	cleanupTicker := time.NewTicker(6 * time.Hour)
	defer pollTicker.Stop()
	defer refreshTicker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			w.pollNext()
		case <-refreshTicker.C:
			w.refreshAccounts()
		case <-cleanupTicker.C:
			w.cleanup()
		}
	}
}

// pollNext fetches the next account in the rotation.
func (w *Watcher) pollNext() {
	acc, ok := w.scheduler.Next()
	if !ok {
		return
	}

	posts, err := w.provider.Fetch(acc)
	if err != nil {
		log.Printf("[social] fetch %s: %v", acc.Handle, err)
		return
	}

	log.Printf("[social] poll %s: %d posts fetched, lastSeen=%s", acc.Handle, len(posts), acc.LastSeenPostID)

	// Cache all fetched posts.
	w.cache.Set(acc.ID, posts)

	// Dedup: find posts we haven't seen yet.
	newPosts := FilterNew(acc, posts)

	if len(newPosts) > 0 {
		log.Printf("[social] %s: %d NEW posts → dispatching", acc.Handle, len(newPosts))
		w.dispatcher.Dispatch(newPosts)

		// Update last seen — both in DB and in the scheduler's in-memory copy
		// so subsequent polls don't re-dispatch the same posts.
		acc.LastSeenPostID = newPosts[0].ID
		acc.LastPolledAt = time.Now().Unix()
		if err := w.accounts.Upsert(acc); err != nil {
			log.Printf("[social] upsert %s: %v", acc.ID, err)
		}
		w.scheduler.UpdateAccount(acc)
	}
}

// refreshAccounts reloads the active account list from the store.
func (w *Watcher) refreshAccounts() {
	accounts, err := w.accounts.GetAllActive()
	if err != nil {
		log.Printf("[social] refresh accounts: %v", err)
		return
	}
	w.scheduler.SetAccounts(accounts)
	log.Printf("[social] refreshed accounts: %d active", len(accounts))
}

// cleanup removes accounts that have not been accessed within cleanupAge.
func (w *Watcher) cleanup() {
	threshold := time.Now().Add(-w.cleanupAge).Unix()
	n, err := w.accounts.CleanupUnused(threshold)
	if err != nil {
		log.Printf("[social] cleanup: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[social] cleaned up %d unused accounts", n)
		w.refreshAccounts()
	}
}
