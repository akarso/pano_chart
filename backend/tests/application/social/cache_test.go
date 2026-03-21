package social_test

import (
	"testing"
	"time"

	appsocial "pano_chart/backend/application/social"
	domain "pano_chart/backend/domain/social"
)

func TestPostCache_SetAndGet(t *testing.T) {
	cache := appsocial.NewPostCache(10 * time.Second)
	posts := []domain.Post{{ID: "1"}, {ID: "2"}}

	cache.Set("acc1", posts)

	got, ok := cache.Get("acc1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(got))
	}
}

func TestPostCache_MissOnEmpty(t *testing.T) {
	cache := appsocial.NewPostCache(10 * time.Second)

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for unknown key")
	}
}

func TestPostCache_ExpiresAfterTTL(t *testing.T) {
	cache := appsocial.NewPostCache(1 * time.Millisecond)
	cache.Set("acc1", []domain.Post{{ID: "1"}})

	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("acc1")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestPostCache_Delete(t *testing.T) {
	cache := appsocial.NewPostCache(10 * time.Second)
	cache.Set("acc1", []domain.Post{{ID: "1"}})

	cache.Delete("acc1")

	_, ok := cache.Get("acc1")
	if ok {
		t.Fatal("expected cache miss after delete")
	}
}

func TestPostCache_Len(t *testing.T) {
	cache := appsocial.NewPostCache(10 * time.Second)
	if cache.Len() != 0 {
		t.Fatalf("expected 0, got %d", cache.Len())
	}

	cache.Set("a", []domain.Post{{ID: "1"}})
	cache.Set("b", []domain.Post{{ID: "2"}})

	if cache.Len() != 2 {
		t.Fatalf("expected 2, got %d", cache.Len())
	}
}

func TestPostCache_OverwriteExistingKey(t *testing.T) {
	cache := appsocial.NewPostCache(10 * time.Second)
	cache.Set("acc1", []domain.Post{{ID: "1"}})
	cache.Set("acc1", []domain.Post{{ID: "2"}, {ID: "3"}})

	got, ok := cache.Get("acc1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 || got[0].ID != "2" {
		t.Fatalf("expected overwritten posts, got %v", got)
	}
}
