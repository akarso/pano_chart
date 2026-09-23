package ports

import (
	"context"
	"time"
)

// ScorecardStore aggregates gradable outcomes for reliability scorecards (PR-092).
// Implementations must INNER JOIN outcomes and exclude administrative rules
// (unsupported, invalid, insufficient_context, path_unavailable).
type ScorecardStore interface {
	// ScorecardAggregate returns per-decile counts for one (kind, label, tf)
	// since `since`, plus same-kind baseline totals excluding that label, in one
	// statement so hit rate and baseline describe the same snapshot.
	ScorecardAggregate(ctx context.Context, kind, label, tf string, since time.Time) (ScorecardAggregate, error)
	// ScorecardSummary returns per-(kind,label) hit totals for a timeframe window.
	ScorecardSummary(ctx context.Context, tf string, since time.Time) ([]ScorecardGroup, error)
}

// ScorecardAggregate is SQL-aggregated scorecard input.
type ScorecardAggregate struct {
	Buckets       [10]ScorecardBucketAgg
	Total         int
	Hits          int
	BaselineHits  int // same kind (+tf), excluding the scored label
	BaselineTotal int
}

// ScorecardBucketAgg is one score-decile aggregate.
type ScorecardBucketAgg struct {
	N         int
	Hits      int
	SumReturn float64
}

// ScorecardGroup is one (kind, label) summary line before baseline attach.
type ScorecardGroup struct {
	Kind  string
	Label string
	Total int
	Hits  int
}
