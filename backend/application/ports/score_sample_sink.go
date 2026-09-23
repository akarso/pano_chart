package ports

import (
	"context"
	"time"
)

// ScoreSampleSink persists sampled calculator scores (PR-094).
type ScoreSampleSink interface {
	Record(ctx context.Context, calculator, symbol, tf string, score float64, at time.Time) error
}

// ScoreSampleReader loads retained scores for one calculator and timeframe.
type ScoreSampleReader interface {
	Scores(ctx context.Context, calculator, tf string) ([]float64, error)
	Calculators(ctx context.Context) ([]string, error)
}
