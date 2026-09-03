package social

import (
	"sync"

	appsocial "pano_chart/backend/application/social"
)

// MemorySubscriptionStore is an in-memory implementation of
// application/social.SubscriptionStore. Suitable for MVP.
type MemorySubscriptionStore struct {
	mu      sync.RWMutex
	subs    map[string]map[string]bool                 // userID → set of accountIDs
	filters map[string]map[string]appsocial.FeedFilter // userID → accountID → filter
}

// NewMemorySubscriptionStore creates an empty in-memory subscription store.
func NewMemorySubscriptionStore() *MemorySubscriptionStore {
	return &MemorySubscriptionStore{
		subs:    make(map[string]map[string]bool),
		filters: make(map[string]map[string]appsocial.FeedFilter),
	}
}

func (s *MemorySubscriptionStore) Subscribe(userID, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs[userID] == nil {
		s.subs[userID] = make(map[string]bool)
	}
	s.subs[userID][accountID] = true
	return nil
}

func (s *MemorySubscriptionStore) Unsubscribe(userID, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs[userID] != nil {
		delete(s.subs[userID], accountID)
	}
	return nil
}

func (s *MemorySubscriptionStore) AccountsForUser(userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0)
	for id := range s.subs[userID] {
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *MemorySubscriptionStore) HasSubscribers(accountID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, accs := range s.subs {
		if accs[accountID] {
			return true, nil
		}
	}
	return false, nil
}

func (s *MemorySubscriptionStore) UsersForAccount(accountID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var users []string
	for userID, accs := range s.subs {
		if accs[accountID] {
			users = append(users, userID)
		}
	}
	return users, nil
}

func (s *MemorySubscriptionStore) SetFilterConfig(userID, accountID string, config appsocial.FeedFilter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filters[userID] == nil {
		s.filters[userID] = make(map[string]appsocial.FeedFilter)
	}
	s.filters[userID][accountID] = config
	return nil
}

func (s *MemorySubscriptionStore) FilterConfigForAccount(accountID string) (map[string]appsocial.FeedFilter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]appsocial.FeedFilter)
	for userID, accs := range s.subs {
		if accs[accountID] {
			if f, ok := s.filters[userID][accountID]; ok {
				result[userID] = f
			} else {
				result[userID] = appsocial.FeedFilter{}
			}
		}
	}
	return result, nil
}
