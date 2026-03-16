package market

import (
	"context"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// RankingsEvaluationProvider adapts the RankingsUseCase to the
// application-layer EvaluationProvider interface.
//
// It executes the rankings pipeline for the requested timeframe and
// converts each RankedResult into an EvaluationSnapshot using the
// component scores already computed by the ranking engine.
type RankingsEvaluationProvider struct {
	rankings usecases.RankingsUseCase
}

// NewRankingsEvaluationProvider constructs the adapter.
func NewRankingsEvaluationProvider(r usecases.RankingsUseCase) *RankingsEvaluationProvider {
	return &RankingsEvaluationProvider{rankings: r}
}

// GetLatestEvaluations implements market.EvaluationProvider.
func (p *RankingsEvaluationProvider) GetLatestEvaluations(timeframe string) ([]domain.EvaluationSnapshot, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return nil, err
	}

	results, err := p.rankings.Execute(context.Background(), usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return nil, err
	}

	snapshots := make([]domain.EvaluationSnapshot, 0, len(results))
	for _, r := range results {
		snapshots = append(snapshots, domain.EvaluationSnapshot{
			Symbol:            r.Symbol.String(),
			Timeframe:         timeframe,
			SidewaysScore:     r.Scores["Sideways Consistency"],
			TrendScore:        r.Scores["Trend Predictability"],
			CompressionScore:  r.Scores["Compression"],
			BreakoutUpScore:   r.Scores["Breakout Up"],
			BreakoutDownScore: r.Scores["Breakout Down"],
		})
	}

	return snapshots, nil
}
