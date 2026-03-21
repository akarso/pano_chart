package social

import "sync"

// MemorySubscriptionStore is an in-memory implementation of
// application/social.SubscriptionStore. Suitable for MVP.
type MemorySubscriptionStore struct {
	mu   sync.RWMutex
	subs map[string]map[string]bool // userID → set of accountIDs
}

// NewMemorySubscriptionStore creates an empty in-memory subscription store.
func NewMemorySubscriptionStore() *MemorySubscriptionStore {
	return &MemorySubscriptionStore{
		subs: make(map[string]map[string]bool),
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
