package social

import (
	"sync"
	"time"

	domain "pano_chart/backend/domain/social"
)

// MemoryAccountStore is an in-memory implementation of
// application/social.AccountStore. Suitable for MVP; swap for SQLite/Redis
// when persistence is needed.
type MemoryAccountStore struct {
	mu   sync.RWMutex
	data map[string]domain.Account
}

// NewMemoryAccountStore creates an empty in-memory account store.
func NewMemoryAccountStore() *MemoryAccountStore {
	return &MemoryAccountStore{
		data: make(map[string]domain.Account),
	}
}

func (s *MemoryAccountStore) Upsert(account domain.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[account.ID] = account
	return nil
}

func (s *MemoryAccountStore) Get(accountID string) (*domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.data[accountID]
	if !ok {
		return nil, nil
	}
	return &acc, nil
}

func (s *MemoryAccountStore) GetAllActive() ([]domain.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Account, 0, len(s.data))
	for _, acc := range s.data {
		result = append(result, acc)
	}
	return result, nil
}

func (s *MemoryAccountStore) MarkUsed(accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.data[accountID]
	if !ok {
		return nil
	}
	acc.LastUsedAt = time.Now().Unix()
	s.data[accountID] = acc
	return nil
}

func (s *MemoryAccountStore) CleanupUnused(thresholdUnix int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, acc := range s.data {
		if acc.LastUsedAt < thresholdUnix {
			delete(s.data, id)
			removed++
		}
	}
	return removed, nil
}
