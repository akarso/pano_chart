package usecases

import (
	"context"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// NewsUseCase defines the boundary for the news feature.
type NewsUseCase interface {
	List(ctx context.Context, limit int) ([]domain.NewsArticle, error)
	GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error)
}

// GetNews implements NewsUseCase by delegating to the repository.
type GetNews struct {
	repo ports.NewsRepository
}

// NewGetNews constructs the GetNews use case.
func NewGetNews(repo ports.NewsRepository) *GetNews {
	return &GetNews{repo: repo}
}

// List returns news articles sorted by date descending.
func (uc *GetNews) List(ctx context.Context, limit int) ([]domain.NewsArticle, error) {
	if limit <= 0 {
		limit = 20
	}
	return uc.repo.List(ctx, limit)
}

// GetBySlug returns a single article by its slug.
func (uc *GetNews) GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error) {
	return uc.repo.GetBySlug(ctx, slug)
}
