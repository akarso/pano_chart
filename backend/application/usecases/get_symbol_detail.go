package usecases

import (
	"context"
	"errors"
	"fmt"

	"pano_chart/backend/application/ports"
	symboluniverse "pano_chart/backend/application/symbol_universe"
	"pano_chart/backend/domain"
)

var ErrSymbolNotFound = errors.New("symbol not found")

// SymbolStats represents scoring results for a symbol.
type SymbolStats struct {
	TotalScore float64
	Scores     map[string]float64
}

// SymbolScorer scores a single symbol series.
type SymbolScorer interface {
	Score(series domain.CandleSeries) (SymbolStats, error)
}

// WeightedSymbolScorer scores a single symbol using weighted calculators.
type WeightedSymbolScorer struct {
	Weights []ScoreWeight
}

func NewWeightedSymbolScorer(weights []ScoreWeight) *WeightedSymbolScorer {
	return &WeightedSymbolScorer{Weights: weights}
}

func (s *WeightedSymbolScorer) Score(series domain.CandleSeries) (SymbolStats, error) {
	scores := make(map[string]float64)
	total := 0.0
	for _, w := range s.Weights {
		if w.Weight == 0 {
			continue
		}
		score, err := w.Calculator.Score(series)
		if err != nil {
			return SymbolStats{}, fmt.Errorf("calculator %s failed: %w", w.Calculator.Name(), err)
		}
		scores[w.Calculator.Name()] = score
		total += score * w.Weight
	}
	return SymbolStats{TotalScore: total, Scores: scores}, nil
}

// GetSymbolDetail provides detailed candle data and optional scores for one symbol.
type GetSymbolDetail struct {
	candleRepo      ports.CandleRepositoryPort
	scorer          SymbolScorer
	universe        symboluniverse.SymbolUniverseProvider
	exchangeInfoURL string
	tickerURL       string
	defaultLimit    int
	maxLimit        int
}

func NewGetSymbolDetail(
	candleRepo ports.CandleRepositoryPort,
	scorer SymbolScorer,
	universe symboluniverse.SymbolUniverseProvider,
	exchangeInfoURL, tickerURL string,
	defaultLimit, maxLimit int,
) *GetSymbolDetail {
	if defaultLimit <= 0 {
		defaultLimit = DefaultSymbolDetailLimit
	}
	if maxLimit <= 0 {
		maxLimit = MaxSymbolDetailLimit
	}
	if defaultLimit > maxLimit {
		defaultLimit = maxLimit
	}
	return &GetSymbolDetail{
		candleRepo:      candleRepo,
		scorer:          scorer,
		universe:        universe,
		exchangeInfoURL: exchangeInfoURL,
		tickerURL:       tickerURL,
		defaultLimit:    defaultLimit,
		maxLimit:        maxLimit,
	}
}

// GetSymbolDetailRequest encapsulates input parameters.
type GetSymbolDetailRequest struct {
	Symbol    domain.Symbol
	Timeframe domain.Timeframe
	Limit     int
}

// SymbolDetailResult is the output DTO for the use case.
type SymbolDetailResult struct {
	Symbol    domain.Symbol
	Timeframe domain.Timeframe
	Candles   []domain.Candle
	Stats     *SymbolStats
}

func (g *GetSymbolDetail) Execute(ctx context.Context, req GetSymbolDetailRequest) (SymbolDetailResult, error) {
	if g.exchangeInfoURL == "" || g.tickerURL == "" {
		return SymbolDetailResult{}, fmt.Errorf("exchangeInfo and ticker URLs are required")
	}
	// 1. Validate symbol exists in universe
	syms, err := g.universe.Symbols(ctx, g.exchangeInfoURL, g.tickerURL)
	if err != nil {
		return SymbolDetailResult{}, fmt.Errorf("universe fetch failed: %w", err)
	}
	found := false
	for _, s := range syms {
		if s == req.Symbol {
			found = true
			break
		}
	}
	if !found {
		return SymbolDetailResult{}, ErrSymbolNotFound
	}

	// 2. Resolve limit
	limit := req.Limit
	if limit <= 0 {
		limit = g.defaultLimit
	}
	if limit > g.maxLimit {
		limit = g.maxLimit
	}

	// 3. Fetch last N candles
	series, err := g.candleRepo.GetLastNCandles(req.Symbol, req.Timeframe, limit)
	if err != nil {
		return SymbolDetailResult{}, fmt.Errorf("candle fetch failed: %w", err)
	}

	candles := make([]domain.Candle, 0, series.Len())
	for i := 0; i < series.Len(); i++ {
		c, err := series.At(i)
		if err != nil {
			continue
		}
		candles = append(candles, c)
	}

	// 4. Compute scores (optional)
	var stats *SymbolStats
	if g.scorer != nil {
		if series.Len() >= 2 {
			computed, err := g.scorer.Score(series)
			if err != nil {
				return SymbolDetailResult{}, fmt.Errorf("score computation failed: %w", err)
			}
			stats = &computed
		} else {
			stats = &SymbolStats{TotalScore: 0, Scores: map[string]float64{}}
		}
	}

	return SymbolDetailResult{
		Symbol:    req.Symbol,
		Timeframe: req.Timeframe,
		Candles:   candles,
		Stats:     stats,
	}, nil
}
