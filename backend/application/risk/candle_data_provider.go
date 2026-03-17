package risk

import (
	"context"
	"math"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/domain"
)

// CandleBasedDataProvider derives approximate risk data from candle series.
// This is a placeholder until real funding/OI/long-short APIs are integrated.
type CandleBasedDataProvider struct {
	candleRepo ports.CandleRepositoryPort
}

// NewCandleBasedDataProvider constructs the provider.
func NewCandleBasedDataProvider(repo ports.CandleRepositoryPort) *CandleBasedDataProvider {
	return &CandleBasedDataProvider{candleRepo: repo}
}

const riskCandleLimit = 50

// Get derives risk-relevant data from candle data.
func (p *CandleBasedDataProvider) Get(ctx context.Context, symbol, timeframe string) (MarketRiskData, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return MarketRiskData{}, err
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return MarketRiskData{}, err
	}

	series, err := p.candleRepo.GetLastNCandles(sym, tf, riskCandleLimit)
	if err != nil {
		return MarketRiskData{}, err
	}

	n := series.Len()
	if n < 2 {
		return MarketRiskData{}, nil
	}

	// Derive approximate price.
	last, _ := series.At(n - 1)
	price := last.Close()

	// Derive OI proxy from volume series (volume expansion ≈ OI expansion).
	oiSeries := make([]float64, n)
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		oiSeries[i] = c.Volume()
	}

	// Derive funding proxy from recent price deviation vs moving average.
	funding := fundingProxy(series)

	// Derive long/short proxy from bullish candle ratio.
	longRatio := bullishRatio(series)

	// Derive nearest cluster proxy from recent high/low extremes.
	cluster := nearestClusterProxy(series, price)

	return MarketRiskData{
		Funding:        funding,
		OISeries:       oiSeries,
		LongRatio:      longRatio,
		Price:          price,
		NearestCluster: cluster,
	}, nil
}

// fundingProxy estimates funding extremeness from price deviation vs SMA.
func fundingProxy(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		sum += c.Close()
	}
	avg := sum / float64(n)
	if avg == 0 {
		return 0
	}
	last, _ := series.At(n - 1)
	// deviation as fraction, clamped to ±0.01
	dev := (last.Close() - avg) / avg
	return dev
}

// bullishRatio returns the fraction of bullish candles.
func bullishRatio(series domain.CandleSeries) float64 {
	n := series.Len()
	if n == 0 {
		return 0.5
	}
	bullish := 0
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		if c.Close() > c.Open() {
			bullish++
		}
	}
	return float64(bullish) / float64(n)
}

// nearestClusterProxy finds the nearest significant price level.
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
