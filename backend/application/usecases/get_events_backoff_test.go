package usecases

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pano_chart/backend/domain"
)

// local fake — mirrors tests/application/usecases but lives next to the
// package so we can ForceExpire white-box (external tests under
// tests/application/usecases cannot see export_test.go helpers).

type backoffFakeProvider struct {
	events []domain.Event
	err    error
	calls  int
}

func (f *backoffFakeProvider) FetchEvents(_ context.Context, _, _ time.Time, _ string) ([]domain.Event, error) {
	f.calls++
	return f.events, f.err
}

func mustEvent(t *testing.T, country, title string, impact domain.EventImpact, ts time.Time) domain.Event {
	t.Helper()
	ev, err := domain.NewEvent("", country, title, impact, ts)
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	return ev
}

func (g *GetEvents) forceExpire(dateFrom, dateTo time.Time, country string) {
	key := cacheKey(dateFrom, dateTo, country)
	g.mu.Lock()
	defer g.mu.Unlock()
	if entry, ok := g.cache[key]; ok {
		entry.fetchedAt = time.Now().Add(-24 * time.Hour)
		entry.errorHoldUntil = time.Time{}
	}
}

// TestGetEvents_ProviderError_ExpiredTTL_BacksOff guards PR-114: after the
// normal TTL expires, a failing FinanceFlow must not be re-hit on every
// subsequent Execute (notification scheduler ticks every 1 minute).
func TestGetEvents_ProviderError_ExpiredTTL_BacksOff(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &backoffFakeProvider{
		events: []domain.Event{
			mustEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
		},
	}

	uc := NewGetEvents(provider)
	req := GetEventsRequest{
		DateFrom: time.Now().UTC().Truncate(24 * time.Hour),
		DateTo:   time.Now().UTC().Add(24 * time.Hour),
		Country:  "United States",
	}

	if _, err := uc.Execute(context.Background(), req); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected 1 seed call, got %d", provider.calls)
	}

	uc.forceExpire(req.DateFrom, req.DateTo, req.Country)
	provider.err = fmt.Errorf("financeflow 503")
	provider.events = nil

	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected stale event, got %d", len(events))
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 provider calls after first failure, got %d", provider.calls)
	}

	for i := 0; i < 5; i++ {
		events, err = uc.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		if len(events) != 1 {
			t.Fatalf("tick %d: expected stale event, got %d", i, len(events))
		}
	}
	if provider.calls != 2 {
		t.Fatalf("expected no further provider calls during backoff, got %d", provider.calls)
	}
}

// TestGetEvents_ProviderError_NoCache_BacksOffEmpty ensures a cold failure
// still caches an empty hold so we do not 1/min retry with nothing.
func TestGetEvents_ProviderError_NoCache_BacksOffEmpty(t *testing.T) {
	provider := &backoffFakeProvider{err: fmt.Errorf("connection refused")}
	uc := NewGetEvents(provider)
	req := GetEventsRequest{
		DateFrom: time.Now().UTC().Truncate(24 * time.Hour),
		DateTo:   time.Now().UTC().Add(24 * time.Hour),
		Country:  "United States",
	}

	events, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty, got %d", len(events))
	}
	if provider.calls != 1 {
		t.Fatalf("expected 1 call, got %d", provider.calls)
	}

	for i := 0; i < 5; i++ {
		_, _ = uc.Execute(context.Background(), req)
	}
	if provider.calls != 1 {
		t.Fatalf("expected backoff to suppress retries, got %d calls", provider.calls)
	}
}
