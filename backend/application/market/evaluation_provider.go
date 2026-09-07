package market

import (
	"context"

	"pano_chart/backend/domain"
)

// EvaluationProvider supplies per-symbol evaluation snapshots
// for a given timeframe. Must honor ctx cancellation — see PR-076 CR
// follow-up: Calculate is called from the notification scheduler's
// background goroutine, and an evaluation fetch that can't be aborted
// keeps Calculate (and therefore the scheduler's Run loop) blocked past
// graceful shutdown's bounded wait for that goroutine, risking a write to
// regimeHistoryRepo after it's been closed.
type EvaluationProvider interface {
	GetLatestEvaluations(ctx context.Context, timeframe string) ([]domain.EvaluationSnapshot, error)
}
