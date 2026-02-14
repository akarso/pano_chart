package usecases

import (
	"context"
	"fmt"
	"sync"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"

	"golang.org/x/sync/errgroup"
)

// GetOverview is a use case that fetches ranked symbols with precomputed sparkline data.
// Sparkline contains close prices only, for efficient frontend rendering.
type GetOverview struct {
	ranker     RankSymbols
	candleRepo ports.CandleRepositoryPort
	precision  int
	maxWorkers int
}

// NewGetOverview constructs the use case.
// precision must be > 0 (caller responsibility to validate at startup).
// maxWorkers controls bounded concurrency when fetching candles.
func NewGetOverview(ranker RankSymbols, candleRepo ports.CandleRepositoryPort, precision int, maxWorkers int) *GetOverview {
	if precision <= 0 {
		precision = 30 // fallback
	}
	if maxWorkers <= 0 {
		maxWorkers = 5
	}
	return &GetOverview{
		ranker:     ranker,
		candleRepo: candleRepo,
		precision:  precision,
		maxWorkers: maxWorkers,
	}
}

// OverviewResult represents a ranked symbol with its sparkline data.
type OverviewResult struct {
	Symbol     domain.Symbol
	TotalScore float64
	Sparkline  []float64 // close prices, chronologically ordered
}

// GetOverviewRequest encapsulates the input parameters for the use case.
type GetOverviewRequest struct {
	Timeframe domain.Timeframe
	Limit     int
}

// Execute fetches ranked symbols and aggregates sparkline data.
// Returns up to Limit ranked symbols with their sparklines.
// If candle fetching fails for a symbol, it is skipped.
// Returns error only if ranking fails or all symbols fail to load candles.
func (g *GetOverview) Execute(ctx context.Context, req GetOverviewRequest) ([]OverviewResult, error) {
	// Rank symbols using the ranking use case.
	// Pass empty series map; ranker will fetch its own.
	ranked, err := g.ranker.Rank(make(map[domain.Symbol]domain.CandleSeries))
	if err != nil {
		return nil, fmt.Errorf("ranking failed: %w", err)
	}

	// Apply limit
	if req.Limit > 0 && len(ranked) > req.Limit {
		ranked = ranked[:req.Limit]
	}

	if len(ranked) == 0 {
		return []OverviewResult{}, nil
	}

	// Fetch candles concurrently with bounded concurrency.
	results := make([]OverviewResult, len(ranked))
	resultsMu := sync.Mutex{}
	successCount := 0

	sem := make(chan struct{}, g.maxWorkers)
	eg, _ := errgroup.WithContext(ctx)

	for i, ranked := range ranked {
		i := i
		rs := ranked

		eg.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			// Fetch last N candles for this symbol.
			series, err := g.candleRepo.GetLastNCandles(rs.Symbol, req.Timeframe, g.precision)
			if err != nil {
				// Log and skip this symbol; do not fail entire overview.
				fmt.Printf("[GetOverview] Error fetching candles for %s: %v\n", rs.Symbol.String(), err)
				return nil // do not propagate error
			}

			// Extract close prices in chronological order.
			sparkline := extractClosePrices(series)

			// Store result.
			resultsMu.Lock()
			results[i] = OverviewResult{
				Symbol:     rs.Symbol,
				TotalScore: rs.TotalScore,
				Sparkline:  sparkline,
			}
			successCount++
			resultsMu.Unlock()

			return nil
		})
	}

	// Wait for all goroutines to finish.
	_ = eg.Wait()

	// Filter out zero-values (skipped symbols and symbols with no candles).
	filtered := make([]OverviewResult, 0, len(results))
	for _, r := range results {
		if r.Symbol != "" && len(r.Sparkline) > 0 {
			filtered = append(filtered, r)
		}
	}

	// If all symbols failed, return error.
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no candles available for any ranked symbol")
	}

	return filtered, nil
}

// extractClosePrices extracts close prices from a CandleSeries in chronological order.
func extractClosePrices(series domain.CandleSeries) []float64 {
	closes := make([]float64, series.Len())
	for i := 0; i < series.Len(); i++ {
		candle, err := series.At(i)
		if err != nil {
			// This should not happen if Len() is accurate.
			continue
		}
		closes[i] = candle.Close()
	}
	return closes
}
