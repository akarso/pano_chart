package market

import "pano_chart/backend/domain"

// EvaluationProvider supplies per-symbol evaluation snapshots
// for a given timeframe.
type EvaluationProvider interface {
	GetLatestEvaluations(timeframe string) ([]domain.EvaluationSnapshot, error)
}
