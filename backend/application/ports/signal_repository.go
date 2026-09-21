package ports

import (
	"context"
	"time"

	"pano_chart/backend/domain/signal"
)

// SignalRepository persists emitted signals and their outcomes.
// See ROADMAP PR-090 / PR-091.
type SignalRepository interface {
	Append(ctx context.Context, s signal.Signal) error
	// Unresolved returns unresolved signals with emitted_at < before (oldest
	// first). Used to drain never-ready rows (invalid TF). Prefer
	// UnresolvedReady for grading.
	Unresolved(ctx context.Context, before time.Time, limit int) ([]signal.Signal, error)
	// UnresolvedReady returns unresolved signals whose horizon has elapsed
	// (emitted_at + horizon_bars×tf_duration ≤ now), oldest first. Unknown
	// timeframes are excluded (they never become ready).
	UnresolvedReady(ctx context.Context, now time.Time, limit int) ([]signal.Signal, error)
	MarkResolved(ctx context.Context, id string, outcome signal.Outcome) error
	Query(ctx context.Context, filter signal.Filter) ([]signal.SignalWithOutcome, error)
}
