package ports

import (
	"context"

	"pano_chart/backend/domain"
)

// NewsRepository provides access to news articles.
type NewsRepository interface {
	// List returns news articles sorted by date descending, limited to the given count.
	List(ctx context.Context, limit int) ([]domain.NewsArticle, error)

	// GetBySlug returns a single article by its slug.
	GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error)
}
