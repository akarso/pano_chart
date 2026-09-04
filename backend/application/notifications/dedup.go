package notifications

import (
	"sync"
	"time"
)

// entry stores a dedup mark with an expiration time.
type entry struct {
	expiresAt time.Time
}

// Deduplicator tracks notification keys to prevent duplicate delivery.
// Keys expire after a configurable TTL; stale entries are evicted lazily.
type Deduplicator struct {
	mu   sync.Mutex
	seen map[string]entry
	now  func() time.Time // injectable clock for testing
}

// NewDeduplicator creates a ready-to-use Deduplicator.
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		seen: make(map[string]entry),
		now:  time.Now,
	}
}

// Seen returns true if the key was marked and has not yet expired.
func (d *Deduplicator) Seen(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	e, ok := d.seen[key]
	if !ok {
		return false
	}
	if d.now().After(e.expiresAt) {
		delete(d.seen, key) // lazy eviction
		return false
	}
	return true
}

// Mark records the key with the given TTL.
func (d *Deduplicator) Mark(key string, ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[key] = entry{expiresAt: d.now().Add(ttl)}
}

// TryReserve atomically checks whether key is unseen and, if so, marks it
// with the given TTL. It returns true if the reservation was acquired
// (i.e. the caller should proceed with delivery) and false if the key was
// already seen and not yet expired.
func (d *Deduplicator) TryReserve(key string, ttl time.Duration) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if e, ok := d.seen[key]; ok && !d.now().After(e.expiresAt) {
		return false
	}
	d.seen[key] = entry{expiresAt: d.now().Add(ttl)}
	return true
}

// Release removes a key's reservation, allowing an immediate retry.
// Call this when delivery fails after a successful TryReserve.
func (d *Deduplicator) Release(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.seen, key)
}

// Len returns the number of non-expired tracked keys (for diagnostics).
func (d *Deduplicator) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	live := 0
	for k, e := range d.seen {
		if now.After(e.expiresAt) {
			delete(d.seen, k)
		} else {
			live++
		}
	}
	return live
}

// SetClock overrides the time source (for testing).
func (d *Deduplicator) SetClock(fn func() time.Time) { d.now = fn }
