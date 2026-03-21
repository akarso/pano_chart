package social

import (
	"sync"
	"time"

	domain "pano_chart/backend/domain/social"
)

type cacheEntry struct {
	posts    []domain.Post
	storedAt time.Time
}

// PostCache is a thread-safe in-memory cache of posts keyed by account ID.
type PostCache struct {
	mu   sync.RWMutex
	data map[string]cacheEntry
	ttl  time.Duration
}

// NewPostCache creates a cache with the given TTL (e.g. 60–90s).
func NewPostCache(ttl time.Duration) *PostCache {
	return &PostCache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
	}
}

// Set stores posts for the given account, replacing any previous entry.
func (c *PostCache) Set(accountID string, posts []domain.Post) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[accountID] = cacheEntry{posts: posts, storedAt: time.Now()}
}

// Get returns cached posts if the entry exists and has not expired.
func (c *PostCache) Get(accountID string) ([]domain.Post, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[accountID]
	if !ok {
		return nil, false
	}
	if time.Since(entry.storedAt) > c.ttl {
		return nil, false
	}
	return entry.posts, true
}

// Delete removes a single cache entry.
func (c *PostCache) Delete(accountID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, accountID)
}

// Len returns the number of cached entries (including expired ones).
func (c *PostCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}
