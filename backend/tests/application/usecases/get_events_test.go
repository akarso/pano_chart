package usecases_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// fakeEventProvider is a test double for ports.EventProviderPort.
type fakeEventProvider struct {
	events []domain.Event
	err    error
	calls  int
}

func (f *fakeEventProvider) FetchEvents(ctx context.Context, dateFrom, dateTo time.Time, country string) ([]domain.Event, error) {
	f.calls++
	return f.events, f.err
}

func makeEvent(t *testing.T, country, title string, impact domain.EventImpact, ts time.Time) domain.Event {
	t.Helper()
	ev, err := domain.NewEvent("", country, title, impact, ts)
	if err != nil {
		t.Fatalf("makeEvent: %v", err)
	}
	return ev
}

func TestGetEvents_Execute_Success(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{
			makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
			makeEvent(t, "US", "PMI", domain.EventImpactLow, ts.Add(time.Hour)),
		},
	}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC),
	}

	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if provider.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.calls)
	}
}

func TestGetEvents_Execute_FilterByImpact(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{
			makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
			makeEvent(t, "US", "PMI", domain.EventImpactLow, ts.Add(time.Hour)),
			makeEvent(t, "DE", "GDP", domain.EventImpactHigh, ts.Add(2*time.Hour)),
		},
	}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC),
		Impact:   "high",
	}

	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 high-impact events, got %d", len(events))
	}
	for _, e := range events {
		if e.Impact() != domain.EventImpactHigh {
			t.Errorf("expected high impact, got %q", e.Impact())
		}
	}
}

func TestGetEvents_Execute_CacheHit(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{
			makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
		},
	}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC), // future → 30min TTL
	}

	// First call - populates cache
	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("expected 1 call, got %d", provider.calls)
	}

	// Second call - should be cached
	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.calls != 1 {
		t.Errorf("expected 1 provider call (cache hit), got %d", provider.calls)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 cached event, got %d", len(events))
	}
}

func TestGetEvents_Execute_ProviderError_EmptyFallback(t *testing.T) {
	provider := &fakeEventProvider{
		err: fmt.Errorf("connection refused"),
	}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC),
	}

	// No cache exists → should return empty list, not error
	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error for non-critical events, got %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events on error with no cache, got %d", len(events))
	}
}

func TestGetEvents_Execute_ProviderError_StaleCacheFallback(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{
			makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
		},
	}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), // past → 6h TTL
		DateTo:   time.Date(2020, 1, 7, 0, 0, 0, 0, time.UTC),
	}

	// First call → populate cache
	_, _ = uc.Execute(context.Background(), req)

	// Now provider fails
	provider.err = fmt.Errorf("timeout")
	provider.events = nil

	// Cache is fresh (within 6h TTL) → should still serve from cache
	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 cached event, got %d", len(events))
	}
}

func TestGetEvents_Execute_DifferentParamsNotCached(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{
			makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
		},
	}

	uc := usecases.NewGetEvents(provider)

	req1 := usecases.GetEventsRequest{
		DateFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC),
	}
	req2 := usecases.GetEventsRequest{
		DateFrom: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
	}

	_, _ = uc.Execute(context.Background(), req1)
	_, _ = uc.Execute(context.Background(), req2)

	if provider.calls != 2 {
		t.Errorf("expected 2 provider calls (different keys), got %d", provider.calls)
	}
}
