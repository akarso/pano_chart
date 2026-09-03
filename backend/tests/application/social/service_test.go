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

func TestService_FilteredFeed_OmitRetweets(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{
		platform: "twitter",
		posts: []domain.Post{
			{ID: "p1", Title: "Original post", IsRetweet: false},
			{ID: "p2", Title: "RT @someone retweet", IsRetweet: true},
			{ID: "p3", Title: "Another original", IsRetweet: false},
		},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	posts, err := svc.FilteredFeed("alice", appsocial.FeedFilter{OmitRetweets: true})
	if err != nil {
		t.Fatalf("filtered feed: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts (no retweets), got %d", len(posts))
	}
	for _, p := range posts {
		if p.IsRetweet {
			t.Fatalf("unexpected retweet in filtered result: %s", p.ID)
		}
	}
}

func TestService_FilteredFeed_MinLength(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{
		platform: "twitter",
		posts: []domain.Post{
			{ID: "p1", Title: "short"},
			{ID: "p2", Title: "this is a longer post with substantial content"},
		},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	posts, err := svc.FilteredFeed("alice", appsocial.FeedFilter{MinLength: 10})
	if err != nil {
		t.Fatalf("filtered feed: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(posts))
	}
	if posts[0].ID != "p2" {
		t.Fatalf("expected p2, got %s", posts[0].ID)
	}
}

func TestService_FilteredFeed_Keywords(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{
		platform: "twitter",
		posts: []domain.Post{
			{ID: "p1", Title: "Bitcoin hits new ATH"},
			{ID: "p2", Title: "Nice weather today"},
			{ID: "p3", Title: "ETH pumping hard"},
		},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	posts, err := svc.FilteredFeed("alice", appsocial.FeedFilter{Keywords: []string{"bitcoin", "eth"}})
	if err != nil {
		t.Fatalf("filtered feed: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts matching keywords, got %d", len(posts))
	}
}

func TestService_FilteredFeed_NoFilter(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{
		platform: "twitter",
		posts: []domain.Post{
			{ID: "p1", Title: "A"},
			{ID: "p2", Title: "B"},
		},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	posts, err := svc.FilteredFeed("alice", appsocial.FeedFilter{})
	if err != nil {
		t.Fatalf("filtered feed: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts (no filter), got %d", len(posts))
	}
}

func TestService_FilteredFeed_CombinedFilters(t *testing.T) {
	accStore := infrasocial.NewMemoryAccountStore()
	subStore := infrasocial.NewMemorySubscriptionStore()
	cache := appsocial.NewPostCache(60 * time.Second)
	provider := &stubProvider{
		platform: "twitter",
		posts: []domain.Post{
			{ID: "p1", Title: "Bitcoin price analysis for today and tomorrow", IsRetweet: false},
			{ID: "p2", Title: "RT @x Bitcoin short", IsRetweet: true},
			{ID: "p3", Title: "BTC", IsRetweet: false},
			{ID: "p4", Title: "Weather is nice today in the city area", IsRetweet: false},
		},
	}

	svc := appsocial.NewService(provider, accStore, subStore, cache)

	// Require: no retweets, min 10 chars, must mention "bitcoin" or "btc"
	posts, err := svc.FilteredFeed("alice", appsocial.FeedFilter{
		OmitRetweets: true,
		MinLength:    10,
		Keywords:     []string{"bitcoin", "btc"},
	})
	if err != nil {
		t.Fatalf("filtered feed: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(posts))
	}
	if posts[0].ID != "p1" {
		t.Fatalf("expected p1, got %s", posts[0].ID)
	}
}
