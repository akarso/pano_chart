package usecases_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// TestGetEvents_ErrorHold_ExpiresAndRefetches (PR-114): after errorBackoff
// elapses, the next Execute must call the provider again (not fall through to
 // a 30m/6h success TTL on the negative-cache entry).
func TestGetEvents_ErrorHold_ExpiresAndRefetches(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		err: fmt.Errorf("financeflow 503"),
	}

	uc := usecases.NewGetEvents(provider)
	uc.SetErrorBackoff(time.Minute)

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc.SetNow(func() time.Time { return now })

	req := usecases.GetEventsRequest{
		DateFrom: now.Truncate(24 * time.Hour),
		DateTo:   now.Add(24 * time.Hour),
		Country:  "United States",
	}

	// Cold fail → empty hold
	events, err := uc.Execute(context.Background(), req)
	if err != nil || len(events) != 0 {
		t.Fatalf("cold fail: err=%v len=%d", err, len(events))
	}
	if provider.calls != 1 {
		t.Fatalf("expected 1 call, got %d", provider.calls)
	}

	// Still inside hold
	now = now.Add(30 * time.Second)
	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("expected soft-hit during hold, got %d calls", provider.calls)
	}

	// Hold elapsed → must refetch
	now = now.Add(time.Minute)
	provider.err = nil
	provider.events = []domain.Event{
		makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts),
	}
	events, err = uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("after hold: %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected refetch after hold, got %d calls", provider.calls)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after recovery, got %d", len(events))
	}
}

// TestGetEvents_SuccessAfterHold_ClearsHoldAndUsesNormalTTL.
func TestGetEvents_SuccessAfterHold_ClearsHoldAndUsesNormalTTL(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{err: fmt.Errorf("503")}

	uc := usecases.NewGetEvents(provider)
	uc.SetErrorBackoff(time.Minute)

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc.SetNow(func() time.Time { return now })

	req := usecases.GetEventsRequest{
		DateFrom: now.Truncate(24 * time.Hour),
		DateTo:   now.Add(24 * time.Hour),
		Country:  "United States",
	}

	_, _ = uc.Execute(context.Background(), req) // empty hold
	now = now.Add(time.Minute + time.Second)
	provider.err = nil
	provider.events = []domain.Event{makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts)}
	_, _ = uc.Execute(context.Background(), req) // success → normal cache
	if provider.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", provider.calls)
	}

	// Within upcoming TTL (30m) — soft hit, no provider call
	now = now.Add(10 * time.Minute)
	provider.err = fmt.Errorf("should not be called")
	events, err := uc.Execute(context.Background(), req)
	if err != nil || len(events) != 1 {
		t.Fatalf("normal TTL hit: err=%v len=%d", err, len(events))
	}
	if provider.calls != 2 {
		t.Fatalf("expected no fetch during normal TTL, got %d", provider.calls)
	}
}

// TestGetEvents_Canceled_DoesNotInstallEmptyHold.
func TestGetEvents_Canceled_DoesNotInstallEmptyHold(t *testing.T) {
	provider := &fakeEventProvider{err: context.Canceled}
	uc := usecases.NewGetEvents(provider)
	uc.SetErrorBackoff(time.Hour) // would hide a bug for a long time

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc.SetNow(func() time.Time { return now })

	req := usecases.GetEventsRequest{
		DateFrom: now.Truncate(24 * time.Hour),
		DateTo:   now.Add(24 * time.Hour),
		Country:  "United States",
	}

	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("expected 1 call, got %d", provider.calls)
	}

	// Immediate next Execute must hit the provider again (no empty hold).
	provider.err = fmt.Errorf("still down")
	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 2 {
		t.Fatalf("cancel must not install empty hold; expected 2 calls, got %d", provider.calls)
	}
}

// TestGetEvents_PastRangeColdFail_RetriesAtErrorBackoffNotPastTTL.
func TestGetEvents_PastRangeColdFail_RetriesAtErrorBackoffNotPastTTL(t *testing.T) {
	provider := &fakeEventProvider{err: fmt.Errorf("timeout")}
	uc := usecases.NewGetEvents(provider)
	uc.SetErrorBackoff(time.Minute)

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc.SetNow(func() time.Time { return now })

	// Fully past range → pastTTL would be 6h without the fix.
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2020, 1, 7, 0, 0, 0, 0, time.UTC),
		Country:  "United States",
	}

	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("expected 1, got %d", provider.calls)
	}

	now = now.Add(30 * time.Second)
	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("inside hold: expected 1, got %d", provider.calls)
	}

	now = now.Add(time.Minute)
	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 2 {
		t.Fatalf("after 15m-class hold on past range: expected 2, got %d (must not wait 6h)", provider.calls)
	}
}

// TestGetEvents_StaleTTLExpire_BacksOffThenServesStale.
func TestGetEvents_StaleTTLExpire_BacksOffThenServesStale(t *testing.T) {
	ts := time.Date(2025, 3, 3, 14, 45, 0, 0, time.UTC)
	provider := &fakeEventProvider{
		events: []domain.Event{makeEvent(t, "US", "CPI", domain.EventImpactHigh, ts)},
	}
	uc := usecases.NewGetEvents(provider)
	uc.SetErrorBackoff(time.Minute)

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc.SetNow(func() time.Time { return now })

	req := usecases.GetEventsRequest{
		DateFrom: now.Truncate(24 * time.Hour),
		DateTo:   now.Add(24 * time.Hour),
		Country:  "United States",
	}

	_, _ = uc.Execute(context.Background(), req)
	if provider.calls != 1 {
		t.Fatalf("seed: %d", provider.calls)
	}

	// Past upcoming TTL (30m)
	now = now.Add(31 * time.Minute)
	provider.err = fmt.Errorf("503")
	provider.events = nil

	events, err := uc.Execute(context.Background(), req)
	if err != nil || len(events) != 1 {
		t.Fatalf("stale after fail: err=%v len=%d", err, len(events))
	}
	if provider.calls != 2 {
		t.Fatalf("expected fail fetch, got %d", provider.calls)
	}

	for i := 0; i < 5; i++ {
		now = now.Add(5 * time.Second)
		_, _ = uc.Execute(context.Background(), req)
	}
	if provider.calls != 2 {
		t.Fatalf("hold must suppress ticks, got %d", provider.calls)
	}
}

// TestGetEvents_Singleflight_CoalescesConcurrentMisses.
func TestGetEvents_Singleflight_CoalescesConcurrentMisses(t *testing.T) {
	var calls atomic.Int32
	provider := &blockingProvider{calls: &calls, delay: 50 * time.Millisecond}

	uc := usecases.NewGetEvents(provider)
	req := usecases.GetEventsRequest{
		DateFrom: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2025, 6, 3, 0, 0, 0, 0, time.UTC),
		Country:  "United States",
	}

	const n = 8
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_, _ = uc.Execute(context.Background(), req)
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected singleflight to coalesce to 1 fetch, got %d", got)
	}
}

type blockingProvider struct {
	calls *atomic.Int32
	delay time.Duration
}

func (b *blockingProvider) FetchEvents(ctx context.Context, _, _ time.Time, _ string) ([]domain.Event, error) {
	b.calls.Add(1)
	select {
	case <-time.After(b.delay):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
