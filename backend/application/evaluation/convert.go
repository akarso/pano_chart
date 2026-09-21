package evaluation

import (
	"time"

	appmarket "pano_chart/backend/application/market"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// SnapshotsFromRankings delegates to application/market so the refresher and
// RankingsEvaluationProvider share one conversion path without
// infrastructure → application/evaluation imports.
func SnapshotsFromRankings(results []usecases.RankedResult, timeframe string, computedAt time.Time) []domain.EvaluationSnapshot {
	return appmarket.SnapshotsFromRankings(results, timeframe, computedAt)
}
