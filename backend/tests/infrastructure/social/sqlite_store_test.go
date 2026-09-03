package social_test

import (
	"database/sql"
	"testing"
	"time"

	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"

	_ "modernc.org/sqlite"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

// ── SQLiteAccountStore tests ────────────────────────────────────────────────

func TestSQLiteAccountStore_UpsertAndGet(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, err := infrasocial.NewSQLiteAccountStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	acc := domain.NewAccount("twitter", "alice")
	acc.LastUsedAt = 1000

	if err := store.Upsert(acc); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.Get("twitter:alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected account")
	}
	if got.Handle != "alice" {
		t.Fatalf("expected 'alice', got '%s'", got.Handle)
	}
	if got.LastUsedAt != 1000 {
		t.Fatalf("expected LastUsedAt 1000, got %d", got.LastUsedAt)
	}
}

func TestSQLiteAccountStore_GetReturnsNilForMissing(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteAccountStoreFromDB(db)

	got, err := store.Get("twitter:nobody")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil")
	}
}

func TestSQLiteAccountStore_GetAllActive(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteAccountStoreFromDB(db)
	subStore, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	_ = store.Upsert(domain.NewAccount("twitter", "alice"))
	_ = store.Upsert(domain.NewAccount("twitter", "bob"))
	_ = store.Upsert(domain.NewAccount("twitter", "orphan")) // no subscriber

	_ = subStore.Subscribe("user1", "twitter:alice")
	_ = subStore.Subscribe("user1", "twitter:bob")

	all, err := store.GetAllActive()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 (not orphan), got %d", len(all))
	}
}

func TestSQLiteAccountStore_MarkUsed(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteAccountStoreFromDB(db)

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

func TestSQLiteAccountStore_CleanupUnused(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteAccountStoreFromDB(db)
	subStore, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	old := domain.NewAccount("twitter", "stale")
	old.LastUsedAt = 100
	_ = store.Upsert(old)

	fresh := domain.NewAccount("twitter", "active")
	fresh.LastUsedAt = time.Now().Unix()
	_ = store.Upsert(fresh)

	// Both need subscriptions so GetAllActive can see the survivor.
	_ = subStore.Subscribe("user1", "twitter:stale")
	_ = subStore.Subscribe("user1", "twitter:active")

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
		t.Fatalf("expected only 'active' to remain")
	}
}

func TestSQLiteAccountStore_UpsertUpdatesExisting(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteAccountStoreFromDB(db)

	acc := domain.NewAccount("twitter", "alice")
	acc.LastSeenPostID = "post1"
	_ = store.Upsert(acc)

	acc.LastSeenPostID = "post2"
	_ = store.Upsert(acc)

	got, _ := store.Get("twitter:alice")
	if got.LastSeenPostID != "post2" {
		t.Fatalf("expected 'post2', got '%s'", got.LastSeenPostID)
	}
}

// ── SQLiteSubscriptionStore tests ───────────────────────────────────────────

func TestSQLiteSubscriptionStore_SubscribeAndList(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, err := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

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

func TestSQLiteSubscriptionStore_Unsubscribe(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Unsubscribe("user1", "twitter:alice")

	ids, _ := store.AccountsForUser("user1")
	if len(ids) != 0 {
		t.Fatalf("expected 0, got %d", len(ids))
	}
}

func TestSQLiteSubscriptionStore_HasSubscribers(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	has, _ := store.HasSubscribers("twitter:alice")
	if has {
		t.Fatal("expected false")
	}

	_ = store.Subscribe("user1", "twitter:alice")

	has, _ = store.HasSubscribers("twitter:alice")
	if !has {
		t.Fatal("expected true")
	}
}

func TestSQLiteSubscriptionStore_IdempotentSubscribe(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Subscribe("user1", "twitter:alice")

	ids, _ := store.AccountsForUser("user1")
	if len(ids) != 1 {
		t.Fatalf("expected 1, got %d", len(ids))
	}
}

func TestSQLiteSubscriptionStore_UsersForAccount(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Subscribe("user2", "twitter:alice")

	users, err := store.UsersForAccount("twitter:alice")
	if err != nil {
		t.Fatalf("users for account: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2, got %d", len(users))
	}
}

func TestSQLiteSubscriptionStore_UserIsolation(t *testing.T) {
	db := openDB(t)
	defer func() { _ = db.Close() }()
	store, _ := infrasocial.NewSQLiteSubscriptionStoreFromDB(db)

	_ = store.Subscribe("user1", "twitter:alice")
	_ = store.Subscribe("user2", "twitter:bob")

	ids1, _ := store.AccountsForUser("user1")
	ids2, _ := store.AccountsForUser("user2")
	if len(ids1) != 1 || len(ids2) != 1 {
		t.Fatalf("expected 1 each, got %d and %d", len(ids1), len(ids2))
	}
}
