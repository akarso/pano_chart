package social_test

import (
	"testing"
	"time"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"
)

// stubProvider implements domain.Provider for testing the Service layer.
type stubProvider struct {
	platform string
	posts    []domain.Post
	err      error
}

func (s *stubProvider) Platform() string { return s.platform }

func (s *stubProvider) Fetch(_ domain.Account) ([]domain.Post, error) {
	return s.posts, s.err
}

func TestService_SubscribeCreatesAccount(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{platform: "twitter"}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	if err := svc.Subscribe("user1", "alice"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Account should exist.
	acc, err := accStore.Get("twitter:alice")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if acc == nil {
		t.Fatal("expected account to be created")
		return
	}
	if acc.Handle != "alice" {
		t.Fatalf("expected handle 'alice', got '%s'", acc.Handle)
	}

	// Subscription should exist.
	ids, err := subStore.AccountsForUser("user1")
	if err != nil {
		t.Fatalf("accounts for user: %v", err)
	}
	if len(ids) != 1 || ids[0] != "twitter:alice" {
		t.Fatalf("unexpected subscription list: %v", ids)
	}
}

func TestService_Unsubscribe(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{platform: "twitter"}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	_ = svc.Subscribe("user1", "bob")
	if err := svc.Unsubscribe("user1", "bob"); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	ids, _ := subStore.AccountsForUser("user1")
	if len(ids) != 0 {
		t.Fatalf("expected empty subscriptions, got %v", ids)
	}
}

func TestService_FeedUsesCache(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)

	provider := &stubProvider{
		platform: "twitter",
		posts:    []domain.Post{{ID: "p1", Title: "hello"}},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	// First call: cache miss → live fetch.
	posts, err := svc.Feed("alice")
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "p1" {
		t.Fatalf("unexpected posts: %v", posts)
	}

	// Change provider response.
	provider.posts = []domain.Post{{ID: "p2", Title: "world"}}

	// Second call: should hit cache (still seeing p1).
	posts, err = svc.Feed("alice")
	if err != nil {
		t.Fatalf("feed (cached): %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "p1" {
		t.Fatalf("expected cached result, got %v", posts)
	}
}

func TestService_AccountsForUser(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{platform: "twitter"}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	_ = svc.Subscribe("user1", "alice")
	_ = svc.Subscribe("user1", "bob")

	ids, err := svc.AccountsForUser("user1")
	if err != nil {
		t.Fatalf("accounts for user: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(ids))
	}
}
