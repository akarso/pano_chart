package setups

import (
	"context"
	"fmt"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/setup"
)

// SetupService orchestrates candle retrieval, score computation, and setup
// evaluation for a single symbol.
type SetupService struct {
	candleRepo ports.CandleRepositoryPort
	scorer     usecases.SymbolScorer
	engine     *Engine
}

const candleLimit = 200

// NewSetupService constructs the service.
func NewSetupService(repo ports.CandleRepositoryPort, scorer usecases.SymbolScorer, eng *Engine) *SetupService {
	return &SetupService{
		candleRepo: repo,
		scorer:     scorer,
		engine:     eng,
	}
}

// Evaluate fetches candles, computes underlying scores, builds a SetupContext,
// and runs the engine.
func (s *SetupService) Evaluate(_ context.Context, symbol, timeframe string) (setup.SetupScores, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid symbol: %w", err)
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("invalid timeframe: %w", err)
	}

	series, err := s.candleRepo.GetLastNCandles(sym, tf, candleLimit)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("candle fetch: %w", err)
	}

	if series.Len() < 2 {
		return setup.SetupScores{
			Symbol:    symbol,
			Timeframe: timeframe,
			Scores:    map[setup.SetupType]float64{},
		}, nil
	}

	stats, err := s.scorer.Score(series)
	if err != nil {
		return setup.SetupScores{}, fmt.Errorf("scoring: %w", err)
	}

	ctx := buildContext(symbol, series, stats)
	result := s.engine.Evaluate(ctx)
	result.Timeframe = timeframe
	return result, nil
}

// buildContext converts raw scoring output and candle data into a SetupContext.
func buildContext(symbol string, series domain.CandleSeries, stats usecases.SymbolStats) SetupContext {
	return SetupContext{
		Symbol:           symbol,
		CompressionScore: stats.Scores["Compression"],
		TrendScore:       stats.Scores["Trend Predictability"],
		RangeScore:       rangeFromSideways(stats.Scores),
		VolumeScore:      volumeScore(series),
		Volatility:       volatilityFromSeries(series),
	}
}

// rangeFromSideways derives a range score from sideways scores.
// Higher sideways consistency implies better range-reversion opportunity.
func rangeFromSideways(scores map[string]float64) float64 {
	// Use the best available sideways score.
	best := 0.0
	for k, v := range scores {
		if k == "Compression" || k == "Trend Predictability" || k == "Gain/Loss" {
			continue
		}
		if v > best {
			best = v
		}
	}
	return best
}

// volumeScore computes a normalised volume score from the series.
// Compares the most recent volume to the series average.
func volumeScore(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0
	}

	var total float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		total += c.Volume()
	}
	avg := total / float64(n)
	if avg == 0 {
		return 0
	}

	last, _ := series.At(n - 1)
	ratio := last.Volume() / avg
	// Normalise: ratio 0→0, ratio 1→0.5, ratio ≥2→1
	return clamp(ratio / 2.0)
}

// volatilityFromSeries computes a normalised volatility score.
// Uses ATR-like measure: average (high-low)/close, normalised to [0,1].
func volatilityFromSeries(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0
	}

	var total float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		if c.Close() == 0 {
			continue
		}
		total += (c.High() - c.Low()) / c.Close()
	}
	avg := total / float64(n)
	// Typical crypto daily range: 0-10% ≈ 0-0.1.
	// Map 0→0, 0.05→0.5, ≥0.1→1.
	return clamp(avg / 0.1)
}
