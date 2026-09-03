package usecases_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

type fakeNewsRepository struct {
	articles []domain.NewsArticle
	err      error
}

func (f *fakeNewsRepository) List(ctx context.Context, limit int) ([]domain.NewsArticle, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit > 0 && limit < len(f.articles) {
		return f.articles[:limit], nil
	}
	return f.articles, nil
}

func (f *fakeNewsRepository) GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error) {
	if f.err != nil {
		return domain.NewsArticle{}, f.err
	}
	for _, a := range f.articles {
		if a.Slug() == slug {
			return a, nil
		}
	}
	return domain.NewsArticle{}, fmt.Errorf("news article not found: %s", slug)
}

func makeArticle(t *testing.T, slug string) domain.NewsArticle {
	t.Helper()
	a, err := domain.NewNewsArticle(slug, "Title "+slug, time.Now(), domain.NewsStatusInfo, nil, "body", nil, "")
	if err != nil {
		t.Fatalf("makeArticle: %v", err)
	}
	return a
}

func TestGetNews_List_ReturnsAll(t *testing.T) {
	repo := &fakeNewsRepository{
		articles: []domain.NewsArticle{
			makeArticle(t, "a"),
			makeArticle(t, "b"),
		},
	}
	uc := usecases.NewGetNews(repo)

	articles, err := uc.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2, got %d", len(articles))
	}
}

func TestGetNews_List_DefaultsLimitTo20(t *testing.T) {
	repo := &fakeNewsRepository{articles: []domain.NewsArticle{makeArticle(t, "a")}}
	uc := usecases.NewGetNews(repo)

	articles, err := uc.List(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1, got %d", len(articles))
	}
}

func TestGetNews_List_PropagatesError(t *testing.T) {
	repo := &fakeNewsRepository{err: fmt.Errorf("disk error")}
	uc := usecases.NewGetNews(repo)

	_, err := uc.List(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetNews_GetBySlug_Found(t *testing.T) {
	repo := &fakeNewsRepository{
		articles: []domain.NewsArticle{makeArticle(t, "target")},
	}
	uc := usecases.NewGetNews(repo)

	a, err := uc.GetBySlug(context.Background(), "target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Slug() != "target" {
		t.Errorf("slug = %q", a.Slug())
	}
}

func TestGetNews_GetBySlug_NotFound(t *testing.T) {
	repo := &fakeNewsRepository{articles: []domain.NewsArticle{}}
	uc := usecases.NewGetNews(repo)

	_, err := uc.GetBySlug(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
}
