package ports

import (
	"context"
	"time"

	"pano_chart/backend/domain/signal"
)

// SignalRepository persists emitted signals and (later) their outcomes.
// See ROADMAP PR-090 / PR-091.
type SignalRepository interface {
	Append(ctx context.Context, s signal.Signal) error
	Unresolved(ctx context.Context, before time.Time, limit int) ([]signal.Signal, error)
	MarkResolved(ctx context.Context, id string, outcome signal.Outcome) error
	Query(ctx context.Context, filter signal.Filter) ([]signal.SignalWithOutcome, error)
}
