package social

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domain "pano_chart/backend/domain/social"
)

// FeedFilter contains optional filters applied to posts before they are
// returned to the caller. Zero-value fields mean "no filter".
type FeedFilter struct {
	OmitRetweets bool     // if true, exclude retweets
	OmitReplies  bool     // if true, exclude replies
	MinLength    int      // minimum Title length (0 = no minimum)
	Keywords     []string // if non-empty, post Title must contain at least one
}

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

// SetFilterConfig stores per-user per-account filter settings.
func (s *Service) SetFilterConfig(userID, handle string, config FeedFilter) error {
	accID := s.provider.Platform() + ":" + handle
	return s.subs.SetFilterConfig(userID, accID, config)
}

// FilteredFeed fetches posts for a handle and applies the given filter.
func (s *Service) FilteredFeed(handle string, filter FeedFilter) ([]domain.Post, error) {
	posts, err := s.Feed(handle)
	if err != nil {
		return nil, err
	}
	return applyFilter(posts, filter), nil
}

// applyFilter returns only the posts that pass all filter criteria.
func applyFilter(posts []domain.Post, f FeedFilter) []domain.Post {
	if !f.OmitRetweets && !f.OmitReplies && f.MinLength <= 0 && len(f.Keywords) == 0 {
		return posts // nothing to filter
	}

	result := make([]domain.Post, 0, len(posts))
	for _, p := range posts {
		if f.OmitRetweets && p.IsRetweet {
			continue
		}
		if f.OmitReplies && p.IsReply {
			continue
		}
		if f.MinLength > 0 && utf8.RuneCountInString(p.Title) < f.MinLength {
			continue
		}
		if len(f.Keywords) > 0 && !matchesAnyKeyword(p.Title, f.Keywords) {
			continue
		}
		result = append(result, p)
	}
	return result
}

// matchesAnyKeyword returns true if text contains at least one keyword
// (case-insensitive).
func matchesAnyKeyword(text string, keywords []string) bool {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
