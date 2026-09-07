package risk

import (
	"context"
	"fmt"
	"math"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// riskCandleLimit bounds the candle fetch used for Price and the
// liquidation-cluster proximity proxy below (item 4 — see PR-081 §4).
const riskCandleLimit = 50

// BinanceFuturesDataProvider implements DataProvider using real Binance
// Futures market data (funding rate, open interest, long/short account
// ratio) for crowding/fragility scoring. This replaces the previous
// CandleBasedDataProvider, which derived all four components as proxies
// from the same OHLCV candles the sideways/setup scorers already consume —
// see PR-081.
//
// LiquidationProximity is the one component this provider does NOT source
// from real data: Binance has no public REST liquidation-heatmap endpoint,
// only a real-time forced-liquidation websocket stream
// (!forceOrder@arr) with no historical backfill or clustering — building
// that is meaningfully more infrastructure than the three REST calls below
// and is scoped to a separate follow-up (see PR-081 §4). Price and the
// liquidation-cluster proxy stay candle-derived, unchanged from the
// provider this replaces.
type BinanceFuturesDataProvider struct {
	futures ports.FuturesDataPort
	candles ports.CandleRepositoryPort
}

// NewBinanceFuturesDataProvider constructs the provider.
func NewBinanceFuturesDataProvider(futures ports.FuturesDataPort, candles ports.CandleRepositoryPort) *BinanceFuturesDataProvider {
	return &BinanceFuturesDataProvider{futures: futures, candles: candles}
}

// Get implements DataProvider. A failure to fetch any of the three real
// futures signals fails the whole call rather than degrading to zeros or a
// proxy — see PR-081 §5: SetupService already treats a non-cancellation
// FragilityProvider error as "degrade gracefully, Crowding stays at its
// zero default" (added in the PR-076 CR round), so this composes with
// existing behavior for free.
func (p *BinanceFuturesDataProvider) Get(ctx context.Context, symbol, timeframe string) (MarketRiskData, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("invalid symbol: %w", err)
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("invalid timeframe: %w", err)
	}

	funding, err := p.futures.FundingRate(ctx, symbol)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("funding rate: %w", err)
	}
	oiSeries, err := p.futures.OpenInterestHistory(ctx, symbol)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("open interest: %w", err)
	}
	longRatio, err := p.futures.LongShortRatio(ctx, symbol)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("long/short ratio: %w", err)
	}

	series, err := p.candles.GetLastNCandles(ctx, sym, tf, riskCandleLimit)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("candle fetch: %w", err)
	}
	if series.Len() < 2 {
		return MarketRiskData{}, fmt.Errorf("insufficient candle data for %s %s", symbol, timeframe)
	}
	last, err := series.At(series.Len() - 1)
	if err != nil {
		return MarketRiskData{}, fmt.Errorf("candle fetch: %w", err)
	}
	price := last.Close()
	cluster := nearestClusterProxy(series, price)

	return MarketRiskData{
		Funding:        funding,
		OISeries:       oiSeries,
		LongRatio:      longRatio,
		Price:          price,
		NearestCluster: cluster,
	}, nil
}

// nearestClusterProxy finds the nearest significant price level — an
// interim proxy for a real liquidation cluster (see this type's doc
// comment). Unchanged from the provider this replaces.
func nearestClusterProxy(series domain.CandleSeries, currentPrice float64) float64 {
	n := series.Len()
	if n == 0 || currentPrice == 0 {
		return currentPrice
	}
	nearest := currentPrice
	minDist := math.MaxFloat64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		for _, level := range []float64{c.High(), c.Low()} {
			dist := math.Abs(level - currentPrice)
			if dist > 0 && dist < minDist {
				minDist = dist
				nearest = level
			}
		}
	}
	return nearest
}
