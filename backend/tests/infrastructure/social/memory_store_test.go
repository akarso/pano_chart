package social_test

import (
	"testing"
	"time"

	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"
)

func TestMemoryAccountStore_UpsertAndGet(t *testing.T) {
	store := infrasocial.NewMemoryAccountStore()
	acc := domain.NewAccount("twitter", "alice")

	if err := store.Upsert(acc); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.Get("twitter:alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected account")
		return
	}
	if got.Handle != "alice" {
		t.Fatalf("expected handle 'alice', got '%s'", got.Handle)
	}
}

func TestMemoryAccountStore_GetReturnsNilForMissing(t *testing.T) {
	store := infrasocial.NewMemoryAccountStore()

	got, err := store.Get("twitter:nobody")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing account")
	}
}

func TestMemoryAccountStore_GetAllActive(t *testing.T) {
	store := infrasocial.NewMemoryAccountStore()
	_ = store.Upsert(domain.NewAccount("twitter", "alice"))
	_ = store.Upsert(domain.NewAccount("twitter", "bob"))

	all, err := store.GetAllActive()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestMemoryAccountStore_MarkUsed(t *testing.T) {
	store := infrasocial.NewMemoryAccountStore()
	acc := domain.NewAccount("twitter", "alice")
	acc.LastUsedAt = 100
	_ = store.Upsert(acc)

	if err := store.MarkUsed("twitter:alice"); err != nil {
		t.Fatalf("mark used: %v", err)
	}

	got, _ := store.Get("twitter:alice")
	if got.LastUsedAt <= 100 {
		t.Fatalf("expected LastUsedAt to be bumped, got %d", got.LastUsedAt)
	}
}

func TestMemoryAccountStore_CleanupUnused(t *testing.T) {
	store := infrasocial.NewMemoryAccountStore()

	old := domain.NewAccount("twitter", "stale")
	old.LastUsedAt = 100
	_ = store.Upsert(old)

	fresh := domain.NewAccount("twitter", "active")
	fresh.LastUsedAt = time.Now().Unix()
	_ = store.Upsert(fresh)

	threshold := time.Now().Add(-1 * time.Hour).Unix()
	n, err := store.CleanupUnused(threshold)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 removed, got %d", n)
	}

	all, _ := store.GetAllActive()
	if len(all) != 1 || all[0].Handle != "active" {
		t.Fatalf("expected only 'active' to remain, got %v", all)
	}
}

func TestMemorySubscriptionStore_SubscribeAndList(t *testing.T) {
	store := infrasocial.NewMemorySubscriptionStore()

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Subscribe("user1", "twitter:bob")

	ids, err := store.AccountsForUser("user1")
	if err != nil {
		t.Fatalf("accounts for user: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2, got %d", len(ids))
	}
}

func TestMemorySubscriptionStore_Unsubscribe(t *testing.T) {
	store := infrasocial.NewMemorySubscriptionStore()

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Unsubscribe("user1", "twitter:alice")

	ids, _ := store.AccountsForUser("user1")
	if len(ids) != 0 {
		t.Fatalf("expected 0, got %d", len(ids))
	}
}

func TestMemorySubscriptionStore_HasSubscribers(t *testing.T) {
	store := infrasocial.NewMemorySubscriptionStore()

	has, _ := store.HasSubscribers("twitter:alice")
	if has {
		t.Fatal("expected false for no subscribers")
	}

	_ = store.Subscribe("user1", "twitter:alice")

	has, _ = store.HasSubscribers("twitter:alice")
	if !has {
		t.Fatal("expected true after subscribe")
	}
}

func TestMemorySubscriptionStore_IdempotentSubscribe(t *testing.T) {
	store := infrasocial.NewMemorySubscriptionStore()

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Subscribe("user1", "twitter:alice")

	ids, _ := store.AccountsForUser("user1")
	if len(ids) != 1 {
		t.Fatalf("expected 1 (idempotent), got %d", len(ids))
	}
}
