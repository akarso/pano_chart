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

// ErrEvaluationStoreUnavailable is returned by readers when Redis transport
// fails (fail-closed). HTTP adapters map this to a stable client message
// instead of embedding transport strings.
var ErrEvaluationStoreUnavailable = errors.New("evaluation store unavailable")

// EvaluationStore persists per-timeframe evaluation snapshots so rankings,
// setups, Market Pulse and notifications can share one compute pass.
// See ROADMAP PR-089a.
type EvaluationStore interface {
	Put(ctx context.Context, tf string, evals []domain.EvaluationSnapshot, computedAt time.Time) error
	Get(ctx context.Context, tf string) ([]domain.EvaluationSnapshot, time.Time, error)
	GetSymbol(ctx context.Context, tf, symbol string) (domain.EvaluationSnapshot, time.Time, error)
}
