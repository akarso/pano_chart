package usecases

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"

	"golang.org/x/sync/singleflight"
)

// EventsUseCase defines the boundary for the events use case.
type EventsUseCase interface {
	Execute(ctx context.Context, req GetEventsRequest) ([]domain.Event, error)
}

// GetEventsRequest holds the query parameters for the events endpoint.
type GetEventsRequest struct {
	DateFrom time.Time
	DateTo   time.Time
	Impact   string // optional: "high", "medium", "low"
	Country  string // optional
}

// cacheEntry stores cached events with expiration metadata.
type cacheEntry struct {
	events    []domain.Event
	fetchedAt time.Time
	dateFrom  time.Time
	dateTo    time.Time
	// errorHoldUntil, when set and still in the future, keeps the entry
	// serving without re-fetching. After it elapses getCached returns nil
	// (force revalidation) — the entry is never treated as a successful
	// fetch for the normal upcoming/past TTL. See PR-114.
	errorHoldUntil time.Time
	// lastAccess is bumped on soft-hit and successful write; eviction is LRU.
	lastAccess time.Time
}

// DefaultEventsCacheMaxEntries bounds in-memory cache keys (public HTTP can
// invent many country|from|to combinations under error holds).
const DefaultEventsCacheMaxEntries = 256

// GetEvents is the use case that fetches, caches, and filters economic events.
type GetEvents struct {
	provider ports.EventProviderPort

	mu    sync.RWMutex
	cache map[string]*cacheEntry
	sf    singleflight.Group

	upcomingTTL  time.Duration
	pastTTL      time.Duration
	errorBackoff time.Duration
	maxEntries   int

	// now is injectable for tests; nil → time.Now. Guarded by mu when set.
	now func() time.Time
}

// NewGetEvents constructs the use case with reasonable cache TTLs.
func NewGetEvents(provider ports.EventProviderPort) *GetEvents {
	return &GetEvents{
		provider:     provider,
		cache:        make(map[string]*cacheEntry),
		upcomingTTL:  30 * time.Minute,
		pastTTL:      6 * time.Hour,
		errorBackoff: 15 * time.Minute,
		maxEntries:   DefaultEventsCacheMaxEntries,
	}
}

// SetErrorBackoff overrides the post-failure hold duration. Test-only;
// zero or negative values are ignored. Synchronized with Execute.
func (g *GetEvents) SetErrorBackoff(d time.Duration) {
	if d <= 0 {
		return
	}
	g.mu.Lock()
	g.errorBackoff = d
	g.mu.Unlock()
}

// SetNow overrides the clock (tests). Pass nil to restore time.Now.
// Synchronized with Execute.
func (g *GetEvents) SetNow(fn func() time.Time) {
	g.mu.Lock()
	g.now = fn
	g.mu.Unlock()
}

// SetMaxEntries overrides the in-memory cache cap (tests).
func (g *GetEvents) SetMaxEntries(n int) {
	if n <= 0 {
		return
	}
	g.mu.Lock()
	g.maxEntries = n
	g.mu.Unlock()
}

func (g *GetEvents) clock() time.Time {
	g.mu.RLock()
	fn := g.now
	g.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now()
}

func (g *GetEvents) backoff() time.Duration {
	g.mu.RLock()
	d := g.errorBackoff
	g.mu.RUnlock()
	return d
}

// eventsFlightResult is the singleflight payload for Execute.
type eventsFlightResult struct {
	events []domain.Event
}

// Execute fetches events, using cache when possible.
// If the external provider fails, cached data is returned if available.
// If no cache exists, an empty slice is returned (never an error for events).
func (g *GetEvents) Execute(ctx context.Context, req GetEventsRequest) ([]domain.Event, error) {
	key := cacheKey(req.DateFrom, req.DateTo, req.Country)

	if entry := g.getCached(key, req.DateFrom, req.DateTo); entry != nil {
		g.logCacheHit(key, entry)
		return filterEvents(entry, req.Impact), nil
	}

	ch := g.sf.DoChan(key, func() (interface{}, error) {
		// Double-check after winning the flight.
		if entry := g.getCached(key, req.DateFrom, req.DateTo); entry != nil {
			return eventsFlightResult{events: entry.events}, nil
		}

		// Detach from any single caller's cancel so one aborted HTTP client
		// cannot abort (or poison) a shared FinanceFlow fetch for siblings.
		// http.Client.Timeout on FinanceFlowClient still applies and surfaces
		// as (wrapped) context.DeadlineExceeded — that IS an upstream blip
		// and must arm an error hold (PR-114 review).
		workCtx := context.WithoutCancel(ctx)

		log.Printf("[Events] cache miss for %s, fetching…", key)
		events, err := g.provider.FetchEvents(workCtx, req.DateFrom, req.DateTo, req.Country)
		if err != nil {
			return g.handleFetchError(key, req, err)
		}

		g.putCache(key, events, req.DateFrom, req.DateTo)
		return eventsFlightResult{events: events}, nil
	})

	select {
	case <-ctx.Done():
		// Caller aborted while waiting — do not write a hold here. The shared
		// flight may still complete and cache for siblings (by design).
		if entry := g.getStaleCached(key); entry != nil {
			log.Printf("[Events] caller canceled for %s, serving stale without hold write", key)
			return filterEvents(entry, req.Impact), nil
		}
		log.Printf("[Events] caller canceled for %s, empty (no hold write)", key)
		return []domain.Event{}, nil
	case res := <-ch:
		if res.Err != nil {
			log.Printf("[Events] flight error for %s: %v", key, res.Err)
			return []domain.Event{}, nil
		}
		fr := res.Val.(eventsFlightResult)
		return filterEventsSlice(fr.events, req.Impact), nil
	}
}

func (g *GetEvents) handleFetchError(key string, req GetEventsRequest, err error) (interface{}, error) {
	// All provider errors arm a hold — including wrapped DeadlineExceeded from
	// http.Client.Timeout (the common FinanceFlow blip). Caller abort is handled
	// only by Execute's select on ctx.Done(), not here.
	log.Printf("[Events] upstream fetch error for %s: %v", key, err)
	if entry := g.getStaleCached(key); entry != nil {
		if g.holdOnError(key, req.DateFrom, req.DateTo) {
			log.Printf("[Events] error-hold soft-hit armed for %s (%s)", key, g.backoff())
		} else {
			log.Printf("[Events] serving fresher cache after failed fetch (no hold) for %s", key)
		}
		return eventsFlightResult{events: entry.events}, nil
	}

	g.putCacheWithErrorHold(key, nil, req.DateFrom, req.DateTo)
	log.Printf("[Events] empty negative-cache hold for %s (%s)", key, g.backoff())
	return eventsFlightResult{}, nil
}

func (g *GetEvents) logCacheHit(key string, entry *cacheEntry) {
	now := g.clock()
	if !entry.errorHoldUntil.IsZero() && now.Before(entry.errorHoldUntil) {
		if len(entry.events) == 0 {
			log.Printf("[Events] empty negative-cache soft-hit for %s", key)
		} else {
			log.Printf("[Events] error-hold soft-hit for %s", key)
		}
		return
	}
	log.Printf("[Events] cache hit for %s", key)
}

// getCached returns cached events if the entry is still valid.
// Soft-hits bump lastAccess (LRU). Hold expiry clears the hold and returns
// nil (revalidate) but keeps a warm entry for getStaleCached — same as
// success TTL expiry. Empty negative-cache entries are deleted on hold expiry.
func (g *GetEvents) getCached(key string, dateFrom, dateTo time.Time) *cacheEntry {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.cache[key]
	if !ok {
		return nil
	}

	now := g.clockLocked()
	if !entry.errorHoldUntil.IsZero() {
		if now.Before(entry.errorHoldUntil) {
			entry.lastAccess = now
			return entry
		}
		// Hold elapsed → force revalidation. Keep non-empty entries so a
		// subsequent fail can re-arm holdOnError on the last known events.
		entry.errorHoldUntil = time.Time{}
		if len(entry.events) == 0 {
			delete(g.cache, key)
		}
		return nil
	}

	ttl := g.ttlForAt(dateFrom, dateTo, now)
	if now.Sub(entry.fetchedAt) > ttl {
		return nil
	}
	entry.lastAccess = now
	return entry
}

func (g *GetEvents) getStaleCached(key string) *cacheEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cache[key]
}

func (g *GetEvents) putCache(key string, events []domain.Event, dateFrom, dateTo time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clockLocked()
	g.cache[key] = &cacheEntry{
		events:     events,
		fetchedAt:  now,
		dateFrom:   dateFrom,
		dateTo:     dateTo,
		lastAccess: now,
	}
	g.evictIfNeededLocked()
}

func (g *GetEvents) putCacheWithErrorHold(key string, events []domain.Event, dateFrom, dateTo time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clockLocked()
	// fetchedAt is already past any normal TTL so after errorHoldUntil
	// elapses, getCached returns nil (retry at errorBackoff, not pastTTL).
	g.cache[key] = &cacheEntry{
		events:         events,
		fetchedAt:      now.Add(-g.pastTTL - time.Second),
		dateFrom:       dateFrom,
		dateTo:         dateTo,
		errorHoldUntil: now.Add(g.errorBackoff),
		lastAccess:     now,
	}
	g.evictIfNeededLocked()
}

// holdOnError arms an error hold on an existing entry. Returns false if the
// entry looks like a racing successful refresh (warm, no hold).
func (g *GetEvents) holdOnError(key string, dateFrom, dateTo time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.cache[key]
	if !ok {
		return false
	}
	now := g.clockLocked()
	ttl := g.ttlForAt(dateFrom, dateTo, now)
	if entry.errorHoldUntil.IsZero() && now.Sub(entry.fetchedAt) <= ttl {
		return false
	}
	entry.errorHoldUntil = now.Add(g.errorBackoff)
	entry.fetchedAt = now.Add(-ttl - time.Second)
	entry.lastAccess = now
	return true
}

func (g *GetEvents) clockLocked() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func (g *GetEvents) evictIfNeededLocked() {
	max := g.maxEntries
	if max <= 0 {
		max = DefaultEventsCacheMaxEntries
	}
	for len(g.cache) > max {
		var oldestKey string
		var oldest time.Time
		first := true
		for k, e := range g.cache {
			score := e.lastAccess
			if score.IsZero() {
				score = e.fetchedAt
			}
			if first || score.Before(oldest) {
				oldest = score
				oldestKey = k
				first = false
			}
		}
		if oldestKey == "" {
			return
		}
		delete(g.cache, oldestKey)
	}
}

func (g *GetEvents) ttlForAt(dateFrom, dateTo, now time.Time) time.Duration {
	if dateTo.After(now.UTC()) {
		return g.upcomingTTL
	}
	return g.pastTTL
}

func cacheKey(dateFrom, dateTo time.Time, country string) string {
	c := strings.ToLower(strings.TrimSpace(country))
	if c == "" {
		c = "_all"
	}
	return fmt.Sprintf("%s|%s|%s",
		c,
		dateFrom.Format("2006-01-02"),
		dateTo.Format("2006-01-02"),
	)
}

func filterEvents(entry *cacheEntry, impact string) []domain.Event {
	if entry == nil {
		return []domain.Event{}
	}
	return filterEventsSlice(entry.events, impact)
}

func filterEventsSlice(events []domain.Event, impact string) []domain.Event {
	if impact == "" {
		return events
	}
	target := domain.ParseEventImpact(impact)
	filtered := make([]domain.Event, 0)
	for _, e := range events {
		if e.Impact() == target {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
