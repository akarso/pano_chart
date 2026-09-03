package social

import (
	"sync"

	domain "pano_chart/backend/domain/social"
)

// Scheduler is a thread-safe rotating queue of accounts. Each call to Next
// returns the next account in the ring, providing a natural rate-limiting
// mechanism: with N accounts and 1 call per second the full cycle takes N
// seconds.
type Scheduler struct {
	mu       sync.Mutex
	accounts []domain.Account
	index    int
}

// NewScheduler creates an empty scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// SetAccounts replaces the account list (typically called after a refresh).
// The index is preserved if still in range, reset to 0 otherwise.
func (s *Scheduler) SetAccounts(accounts []domain.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = accounts
	if s.index >= len(accounts) {
		s.index = 0
	}
}

// Next returns the next account in the rotation. Returns false when there
// are no accounts.
func (s *Scheduler) Next() (domain.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.accounts) == 0 {
		return domain.Account{}, false
	}
	acc := s.accounts[s.index]
	s.index = (s.index + 1) % len(s.accounts)
	return acc, true
}

// UpdateAccount patches the in-memory copy so that subsequent Next calls
// return the updated state without waiting for a full refresh.
func (s *Scheduler) UpdateAccount(updated domain.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == updated.ID {
			s.accounts[i] = updated
			return
		}
	}
}

// Len returns the current number of accounts.
func (s *Scheduler) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.accounts)
}
