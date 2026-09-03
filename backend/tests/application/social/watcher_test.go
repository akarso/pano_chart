package social_test

import (
	"context"
	"sync"
	"testing"
	"time"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"
)

// countingProvider counts Fetch calls per handle.
type countingProvider struct {
	mu     sync.Mutex
	counts map[string]int
}

func newCountingProvider() *countingProvider {
	return &countingProvider{counts: make(map[string]int)}
}

func (p *countingProvider) Platform() string { return "twitter" }

func (p *countingProvider) Fetch(acc domain.Account) ([]domain.Post, error) {
	p.mu.Lock()
	p.counts[acc.Handle]++
	p.mu.Unlock()
	return nil, nil
}

func (p *countingProvider) fetchCount(handle string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[handle]
}

func TestWatcher_AccountCooldown_SkipsRecentlyPolled(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := newCountingProvider()
	dispatcher := appsocial.NewDispatcher(16)

	// Seed two accounts.
	_ = accStore.Upsert(domain.NewAccount("twitter", "alice"))
	_ = accStore.Upsert(domain.NewAccount("twitter", "bob"))

	cfg := appsocial.WatcherConfig{
		PollInterval:    10 * time.Millisecond,
		RefreshInterval: 10 * time.Minute, // won't trigger during test
		CleanupAge:      24 * time.Hour,
		AccountCooldown: 200 * time.Millisecond,
	}

	w := appsocial.NewWatcher(provider, cache, accStore, subStore, dispatcher, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	go w.Run(ctx)

	// Wait long enough for many poll ticks but only ~2 cooldown windows.
	time.Sleep(500 * time.Millisecond)
	cancel()

	// With 200ms cooldown and 500ms runtime, each account should be polled
	// roughly 2–3 times, NOT 50 times (which 10ms tick × 500ms would give
	// without per-account cooldown).
	aliceCount := provider.fetchCount("alice")
	bobCount := provider.fetchCount("bob")

	t.Logf("alice=%d bob=%d", aliceCount, bobCount)

	if aliceCount > 6 {
		t.Errorf("alice polled %d times in 500ms with 200ms cooldown; expected ≤6", aliceCount)
	}
	if bobCount > 6 {
		t.Errorf("bob polled %d times in 500ms with 200ms cooldown; expected ≤6", bobCount)
	}
	// Both should have been polled at least once.
	if aliceCount == 0 {
		t.Error("alice was never polled")
	}
	if bobCount == 0 {
		t.Error("bob was never polled")
	}
}

func TestWatcher_ZeroCooldown_PollsEveryTick(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := newCountingProvider()
	dispatcher := appsocial.NewDispatcher(16)

	_ = accStore.Upsert(domain.NewAccount("twitter", "alice"))

	cfg := appsocial.WatcherConfig{
		PollInterval:    10 * time.Millisecond,
		RefreshInterval: 10 * time.Minute,
		CleanupAge:      24 * time.Hour,
		AccountCooldown: 0, // no cooldown
	}

	w := appsocial.NewWatcher(provider, cache, accStore, subStore, dispatcher, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	count := provider.fetchCount("alice")
	t.Logf("alice=%d (zero cooldown)", count)

	// With 10ms tick and no cooldown, should get polled many times.
	if count < 10 {
		t.Errorf("expected alice polled ≥10 times with zero cooldown, got %d", count)
	}
}
