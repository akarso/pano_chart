package market

import (
	"context"
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RankingsEvaluationProvider adapts the RankingsUseCase to the
// application-layer EvaluationProvider interface.
//
// It executes the rankings pipeline for the requested timeframe and
// converts via application/market.SnapshotsFromRankings (shared with the
// eval store writer — no application/evaluation import).
type RankingsEvaluationProvider struct {
	rankings usecases.RankingsUseCase
}

// NewRankingsEvaluationProvider constructs the adapter.
func NewRankingsEvaluationProvider(r usecases.RankingsUseCase) *RankingsEvaluationProvider {
	return &RankingsEvaluationProvider{rankings: r}
}

// GetLatestEvaluations implements market.EvaluationProvider. Forwards ctx
// into the rankings pipeline (already fully cancellation-aware end to
// end — universe/volume fetch, per-symbol candle fetch, and the bounded
// worker pool all take ctx) instead of the context.Background() this used
// to hardcode, so a cancelled ctx actually aborts an in-flight rankings
// run — see PR-076 CR follow-up.
func (p *RankingsEvaluationProvider) GetLatestEvaluations(ctx context.Context, timeframe string) ([]domain.EvaluationSnapshot, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return nil, err
	}

	results, err := p.rankings.Execute(ctx, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return nil, err
	}

	// Zero computedAt: on-demand path is not a store write (ComputedAt stays 0).
	return appmarket.SnapshotsFromRankings(results, timeframe, time.Time{}), nil
}
