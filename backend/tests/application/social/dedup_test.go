package social_test

import (
	"testing"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
)

func TestFilterNew_AllNewWhenNoLastSeen(t *testing.T) {
	acc := domain.Account{ID: "twitter:alice", LastSeenPostID: ""}
	posts := []domain.Post{
		{ID: "3"}, {ID: "2"}, {ID: "1"},
	}

	got := appsocial.FilterNew(acc, posts)
	if len(got) != 3 {
		t.Fatalf("expected 3 new posts, got %d", len(got))
	}
}

func TestFilterNew_StopsAtLastSeen(t *testing.T) {
	acc := domain.Account{ID: "twitter:alice", LastSeenPostID: "2"}
	posts := []domain.Post{
		{ID: "4"}, {ID: "3"}, {ID: "2"}, {ID: "1"},
	}

	got := appsocial.FilterNew(acc, posts)
	if len(got) != 2 {
		t.Fatalf("expected 2 new posts, got %d", len(got))
	}
	if got[0].ID != "4" || got[1].ID != "3" {
		t.Fatalf("unexpected posts: %v", got)
	}
}

func TestFilterNew_NoneNewWhenLastSeenIsFirst(t *testing.T) {
	acc := domain.Account{ID: "twitter:alice", LastSeenPostID: "3"}
	posts := []domain.Post{
		{ID: "3"}, {ID: "2"}, {ID: "1"},
	}

	got := appsocial.FilterNew(acc, posts)
	if len(got) != 0 {
		t.Fatalf("expected 0 new posts, got %d", len(got))
	}
}

func TestFilterNew_AllNewWhenLastSeenNotFound(t *testing.T) {
	acc := domain.Account{ID: "twitter:alice", LastSeenPostID: "missing"}
	posts := []domain.Post{
		{ID: "3"}, {ID: "2"}, {ID: "1"},
	}

	got := appsocial.FilterNew(acc, posts)
	if len(got) != 3 {
		t.Fatalf("expected 3 posts (last seen not in list), got %d", len(got))
	}
}

func TestFilterNew_EmptyPosts(t *testing.T) {
	acc := domain.Account{ID: "twitter:alice", LastSeenPostID: "1"}

	got := appsocial.FilterNew(acc, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(got))
	}
}
