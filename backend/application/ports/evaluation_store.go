package ports

import (
	"context"
	"errors"
	"time"

	"pano_chart/backend/domain"
)

// ErrEvaluationNotFound is returned when the store has no data for the
// requested timeframe / symbol.
var ErrEvaluationNotFound = errors.New("evaluation store: not found")

// EvaluationStore persists per-timeframe evaluation snapshots so rankings,
// setups, Market Pulse and notifications can share one compute pass.
// See ROADMAP PR-089a.
type EvaluationStore interface {
	Put(ctx context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error
	Get(ctx context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error)
	GetSymbol(ctx context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error)
}
