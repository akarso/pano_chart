package notifications

import (
	"context"
	"log"

	appnotify "pano_chart/backend/application/notifications"
	"pano_chart/backend/application/setups"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/setup"
)

// Compile-time check: SetupScanAdapter implements appnotify.SetupProvider.
var _ appnotify.SetupProvider = (*SetupScanAdapter)(nil)

// RankingsProvider returns pre-scored symbols (typically cached).
type RankingsProvider interface {
	Execute(ctx context.Context, req usecases.GetRankingsRequest) ([]usecases.RankedResult, error)
}

// SetupScanAdapter wraps the setup service and uses pre-cached rankings to
// scan the top symbols and find the best setup for a given timeframe.
type SetupScanAdapter struct {
	setupSvc *setups.SetupService
	rankings RankingsProvider
}

// NewSetupScanAdapter creates an adapter that implements appnotify.SetupProvider.
func NewSetupScanAdapter(setupSvc *setups.SetupService, rankings RankingsProvider) *SetupScanAdapter {
	return &SetupScanAdapter{setupSvc: setupSvc, rankings: rankings}
}

// scanLimit is the maximum number of top-ranked symbols to evaluate.
const scanLimit = 20

// BestSetup evaluates the top-ranked symbols and returns the one with the
// highest setup score. Returns zero-value SetupScores if nothing qualifies.
func (a *SetupScanAdapter) BestSetup(ctx context.Context, timeframe string) (setup.SetupScores, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return setup.SetupScores{}, err
	}

	results, err := a.rankings.Execute(ctx, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return setup.SetupScores{}, err
	}

	n := scanLimit
	if len(results) < n {
		n = len(results)
	}

	var best setup.SetupScores
	for _, r := range results[:n] {
		scores, err := a.setupSvc.Evaluate(ctx, string(r.Symbol), timeframe)
		if err != nil {
			log.Printf("[notify-setup-scan] eval %s/%s error: %v", r.Symbol, timeframe, err)
			continue
		}
		if scores.Score > best.Score {
			best = scores
		}
	}

	if best.Score > 0 {
		log.Printf("[notify-setup-scan] best=%s score=%.2f confidence=%.2f tf=%s",
			best.Symbol, best.Score, best.Confidence, timeframe)
	}

	return best, nil
}
