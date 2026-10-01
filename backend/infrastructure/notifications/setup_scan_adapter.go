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
	Execute(ctx context.Context, req usecases.GetRankingsRequest) (usecases.RankingsResult, error)
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
// highest setup score. SymbolAlertFields are taken from the winning
// rankings row so PR-102 alert context still has score/RS/sparkline on
// 1m/5m (evaluation store is never written for those timeframes).
func (a *SetupScanAdapter) BestSetup(ctx context.Context, timeframe string) (setup.SetupScores, appnotify.SymbolAlertFields, error) {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return setup.SetupScores{}, appnotify.SymbolAlertFields{}, err
	}

	out, err := a.rankings.Execute(ctx, usecases.GetRankingsRequest{
		Timeframe: tf,
		Sort:      usecases.SortByTotal,
	})
	if err != nil {
		return setup.SetupScores{}, appnotify.SymbolAlertFields{}, err
	}
	results := out.Results

	n := scanLimit
	if len(results) < n {
		n = len(results)
	}

	var best setup.SetupScores
	var bestRanked *usecases.RankedResult
	for i := range results[:n] {
		r := &results[i]
		scores, err := a.setupSvc.Evaluate(ctx, string(r.Symbol), timeframe)
		if err != nil {
			log.Printf("[notify-setup-scan] eval %s/%s error: %v", r.Symbol, timeframe, err)
			continue
		}
		if scores.Score > best.Score {
			best = scores
			bestRanked = r
		}
	}

	if best.Score > 0 {
		log.Printf("[notify-setup-scan] best=%s score=%.2f confidence=%.2f tf=%s",
			best.Symbol, best.Score, best.Confidence, timeframe)
	}

	return best, symbolFieldsFromRanked(bestRanked), nil
}

func symbolFieldsFromRanked(r *usecases.RankedResult) appnotify.SymbolAlertFields {
	if r == nil {
		return appnotify.SymbolAlertFields{}
	}
	score := r.TotalScore
	fields := appnotify.SymbolAlertFields{
		TotalScore: &score,
		Sparkline:  append([]float64(nil), r.Sparkline...),
	}
	if r.RelativeStrength != nil {
		rs := *r.RelativeStrength
		fields.RS = &rs
	}
	return fields
}
