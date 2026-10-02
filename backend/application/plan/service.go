package plan

import (
	"context"
	"errors"
	"fmt"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// ErrDataUnavailable is returned when candles cannot be loaded or are empty.
var ErrDataUnavailable = errors.New("plan data unavailable")

// Service builds range plans from live candles.
type Service struct {
	candles ports.CandleRepositoryPort
}

// NewService constructs a plan Service.
func NewService(candles ports.CandleRepositoryPort) *Service {
	return &Service{candles: candles}
}

// PlanResult is a RangePlan plus optional position sizes for the given risk.
type PlanResult struct {
	Plan      RangePlan
	Size      float64 // long size from Size(risk, LongEntry, LongStop)
	ShortSize float64
}

// Evaluate fetches WindowBars(timeframe) candles (Sideways CandleCount) and
// builds a RangePlan. risk ≤ 0 skips size computation (sizes stay 0).
func (s *Service) Evaluate(ctx context.Context, symbol, timeframe string, risk float64) (PlanResult, error) {
	if s == nil || s.candles == nil {
		return PlanResult{}, fmt.Errorf("plan service not configured")
	}
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return PlanResult{}, fmt.Errorf("symbol: %w", err)
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return PlanResult{}, fmt.Errorf("timeframe: %w", err)
	}
	series, err := s.candles.GetLastNCandles(ctx, sym, tf, WindowBars(tf.String()))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return PlanResult{}, err
		}
		return PlanResult{}, fmt.Errorf("%w: %v", ErrDataUnavailable, err)
	}
	if series.Len() == 0 {
		return PlanResult{}, ErrDataUnavailable
	}
	p := BuildRangePlan(sym.String(), tf.String(), series)
	out := PlanResult{Plan: p}
	if risk > 0 && p.Valid {
		out.Size = Size(risk, p.LongEntry, p.LongStop)
		out.ShortSize = Size(risk, p.ShortEntry, p.ShortStop)
	}
	return out, nil
}
