package social

import (
	"fmt"
	"time"

	domain "pano_chart/backend/domain/social"
)

// Service is the application facade for social features.
// HTTP handlers call this; it coordinates stores, cache, and provider.
type Service struct {
	provider domain.Provider
	accounts AccountStore
	subs     SubscriptionStore
	cache    *PostCache
}

// NewService creates a social application service.
func NewService(
	provider domain.Provider,
	accounts AccountStore,
	subs SubscriptionStore,
	cache *PostCache,
) *Service {
	return &Service{
		provider: provider,
		accounts: accounts,
		subs:     subs,
		cache:    cache,
	}
}

// Subscribe adds a user subscription to a handle. Creates the account if it
// does not yet exist.
func (s *Service) Subscribe(userID, handle string) error {
	acc := domain.NewAccount(s.provider.Platform(), handle)
	acc.LastUsedAt = time.Now().Unix()

	if err := s.accounts.Upsert(acc); err != nil {
		return fmt.Errorf("upsert account: %w", err)
	}
	if err := s.subs.Subscribe(userID, acc.ID); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	return nil
}

// Unsubscribe removes a user subscription.
func (s *Service) Unsubscribe(userID, handle string) error {
	accID := s.provider.Platform() + ":" + handle
	return s.subs.Unsubscribe(userID, accID)
}

// Feed returns posts for a handle. Tries the cache first; on a miss it does
// a live fetch through the provider.
func (s *Service) Feed(handle string) ([]domain.Post, error) {
	accID := s.provider.Platform() + ":" + handle

	// Try cache first.
	if posts, ok := s.cache.Get(accID); ok {
		return posts, nil
	}

	// Cache miss: fetch live.
	acc, err := s.accounts.Get(accID)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		// Account not tracked yet — create ephemeral account for one-off fetch.
		tmp := domain.NewAccount(s.provider.Platform(), handle)
		acc = &tmp
	}

	posts, err := s.provider.Fetch(*acc)
	if err != nil {
		return nil, err
	}

	s.cache.Set(accID, posts)

	// Bump usage.
	_ = s.accounts.MarkUsed(accID)

	return posts, nil
}

// AccountsForUser returns the account IDs a user is subscribed to.
func (s *Service) AccountsForUser(userID string) ([]string, error) {
	return s.subs.AccountsForUser(userID)
}
