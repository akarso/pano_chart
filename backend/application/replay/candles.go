// Package replay supports PR-112a as-of market reads: candle windows ending
// at a past timestamp, with live caches bypassed by callers that check AsOf.
package replay

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

type ctxKey struct{}

// WithAsOf attaches a replay cutoff (unix-second instant) to ctx.
func WithAsOf(ctx context.Context, asOf time.Time) context.Context {
	return context.WithValue(ctx, ctxKey{}, asOf.UTC())
}

// AsOf returns the replay cutoff when present.
func AsOf(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(ctxKey{}).(time.Time)
	return t, ok
}

// ParseUnixSeconds parses asOf=<unix seconds> from a query value.
// Empty string means no replay. Rejects non-positive / unparseable values.
func ParseUnixSeconds(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	sec, err := parseInt64(raw)
	if err != nil || sec <= 0 {
		return nil, fmt.Errorf("invalid asOf")
	}
	t := time.Unix(sec, 0).UTC()
	return &t, nil
}

// ValidateAsOf rejects replay cutoffs in the future relative to now.
func ValidateAsOf(asOf time.Time, now time.Time) error {
	if asOf.After(now.UTC()) {
		return fmt.Errorf("invalid asOf")
	}
	return nil
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// FetchCandles returns the last n completed candles. Live path delegates to
// GetLastNCandles (which drops the in-progress bar). Replay path uses
// GetSeries then keeps only bars with open+tf <= asOf so final OHLC from a
// still-forming bar at asOf cannot leak in (PR-112a).
func FetchCandles(
	ctx context.Context,
	repo ports.CandleRepositoryPort,
	sym domain.Symbol,
	tf domain.Timeframe,
	n int,
) (domain.CandleSeries, error) {
	if repo == nil {
		return domain.CandleSeries{}, fmt.Errorf("candle repository not configured")
	}
	if n <= 0 {
		return domain.CandleSeries{}, fmt.Errorf("n must be positive")
	}
	if asOf, ok := AsOf(ctx); ok {
		return fetchCandlesAsOf(ctx, repo, sym, tf, n, asOf)
	}
	return repo.GetLastNCandles(ctx, sym, tf, n)
}

func fetchCandlesAsOf(
	ctx context.Context,
	repo ports.CandleRepositoryPort,
	sym domain.Symbol,
	tf domain.Timeframe,
	n int,
	asOf time.Time,
) (domain.CandleSeries, error) {
	dur := tf.Duration()
	if dur <= 0 {
		return domain.CandleSeries{}, fmt.Errorf("invalid timeframe duration")
	}
	asOf = asOf.UTC()
	// Fetch one extra bar window so adapters that return the forming bar can
	// be filtered out locally regardless of inclusive/exclusive endTime.
	from := asOf.Add(-time.Duration(n+2) * dur)
	to := asOf.Add(dur)
	series, err := repo.GetSeries(ctx, sym, tf, from, to)
	if err != nil {
		return domain.CandleSeries{}, err
	}
	closed := filterCompletedThrough(series, asOf, dur)
	if len(closed) > n {
		closed = closed[len(closed)-n:]
	}
	return domain.NewCandleSeries(sym, tf, closed)
}

// filterCompletedThrough keeps bars fully closed at asOf (open+dur <= asOf).
func filterCompletedThrough(series domain.CandleSeries, asOf time.Time, dur time.Duration) []domain.Candle {
	if series.Len() == 0 {
		return nil
	}
	out := make([]domain.Candle, 0, series.Len())
	for i := 0; i < series.Len(); i++ {
		c, err := series.At(i)
		if err != nil {
			continue
		}
		closeAt := c.Timestamp().Add(dur)
		if closeAt.After(asOf) {
			continue
		}
		out = append(out, c)
	}
	return out
}
