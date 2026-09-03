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
}

// GetEvents is the use case that fetches, caches, and filters economic events.
type GetEvents struct {
	provider ports.EventProviderPort

	mu    sync.RWMutex
	cache map[string]*cacheEntry

	upcomingTTL time.Duration // TTL for date ranges that include future dates
	pastTTL     time.Duration // TTL for fully-past date ranges
}

// NewGetEvents constructs the use case with reasonable cache TTLs.
func NewGetEvents(provider ports.EventProviderPort) *GetEvents {
	return &GetEvents{
		provider:    provider,
		cache:       make(map[string]*cacheEntry),
		upcomingTTL: 30 * time.Minute,
		pastTTL:     6 * time.Hour,
	}
}

// Execute fetches events, using cache when possible.
// If the external provider fails, cached data is returned if available.
// If no cache exists, an empty slice is returned (never an error for events).
func (g *GetEvents) Execute(ctx context.Context, req GetEventsRequest) ([]domain.Event, error) {
	key := cacheKey(req.DateFrom, req.DateTo, req.Country)

	// Try cache first
	if entry := g.getCached(key, req.DateFrom, req.DateTo); entry != nil {
		log.Printf("[Events] cache hit for %s", key)
		return filterEvents(entry, req.Impact), nil
	}

	log.Printf("[Events] cache miss for %s, fetching…", key)
	events, err := g.provider.FetchEvents(ctx, req.DateFrom, req.DateTo, req.Country)
	if err != nil {
		log.Printf("[Events] fetch error: %v, falling back to stale cache", err)
		// Return stale cache if available (ignore TTL on error)
		if entry := g.getStaleCached(key); entry != nil {
			return filterEvents(entry, req.Impact), nil
		}
		// Non-critical: return empty list, not 500
		return []domain.Event{}, nil
	}

	g.putCache(key, events, req.DateFrom, req.DateTo)
	return filterEventsSlice(events, req.Impact), nil
}

// getCached returns cached events if the entry exists and hasn't expired.
func (g *GetEvents) getCached(key string, dateFrom, dateTo time.Time) *cacheEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	entry, ok := g.cache[key]
	if !ok {
		return nil
	}

	ttl := g.ttlFor(dateFrom, dateTo)
	if time.Since(entry.fetchedAt) > ttl {
		return nil
	}
	return entry
}

// getStaleCached returns cached events regardless of TTL (for error fallback).
func (g *GetEvents) getStaleCached(key string) *cacheEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	entry, ok := g.cache[key]
	if !ok {
		return nil
	}
	return entry
}

func (g *GetEvents) putCache(key string, events []domain.Event, dateFrom, dateTo time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.cache[key] = &cacheEntry{
		events:    events,
		fetchedAt: time.Now(),
		dateFrom:  dateFrom,
		dateTo:    dateTo,
	}
}

// ttlFor returns the appropriate TTL for a date range.
// If the range extends into the future, use the shorter upcoming TTL.
func (g *GetEvents) ttlFor(dateFrom, dateTo time.Time) time.Duration {
	now := time.Now().UTC()
	if dateTo.After(now) {
		return g.upcomingTTL
	}
	return g.pastTTL
}

// cacheKey builds a deterministic key from the query parameters.
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

// filterEvents applies optional impact filtering.
func filterEvents(entry *cacheEntry, impact string) []domain.Event {
	if entry == nil {
		return []domain.Event{}
	}
	if impact == "" {
		return entry.events
	}
	target := domain.ParseEventImpact(impact)
	filtered := make([]domain.Event, 0)
	for _, e := range entry.events {
		if e.Impact() == target {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// filterEventsSlice applies optional impact filtering to a raw slice.
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
