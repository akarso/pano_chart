package behavior

import (
	"context"
	"math"

	"pano_chart/backend/application/ports"
	apprisk "pano_chart/backend/application/risk"
	"pano_chart/backend/domain"
)

// CandleBasedDataProvider derives behavior-relevant signals from candle series.
// This is a placeholder until real funding/OI/regime APIs are integrated.
type CandleBasedDataProvider struct {
	candleRepo ports.CandleRepositoryPort
	riskEngine *apprisk.Engine
}

// NewCandleBasedDataProvider constructs the provider.
func NewCandleBasedDataProvider(repo ports.CandleRepositoryPort, riskEngine *apprisk.Engine) *CandleBasedDataProvider {
	return &CandleBasedDataProvider{candleRepo: repo, riskEngine: riskEngine}
}

const behaviorCandleLimit = 50

// Get derives behavior-relevant data from candle data.
func (p *CandleBasedDataProvider) Get(ctx context.Context, symbol, timeframe string) (BehaviorData, error) {
	sym, err := domain.NewSymbol(symbol)
	if err != nil {
		return BehaviorData{}, err
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return BehaviorData{}, err
	}

	series, err := p.candleRepo.GetLastNCandles(sym, tf, behaviorCandleLimit)
	if err != nil {
		return BehaviorData{}, err
	}

	n := series.Len()
	if n < 2 {
		return BehaviorData{}, nil
	}

	// Reuse risk engine to get fragility components.
	last, _ := series.At(n - 1)
	price := last.Close()

	oiSeries := make([]float64, n)
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		oiSeries[i] = c.Volume()
	}

	funding := fundingProxy(series)
	longRatio := bullishRatio(series)
	cluster := nearestClusterProxy(series, price)

	comps := p.riskEngine.Calculate(funding, oiSeries, longRatio, price, cluster)
	fragilityScore := apprisk.FinalScore(comps)

	// Derive volatility from ATR / price.
	volatility := atrVolatility(series, price)

	// Derive volume score (recent volume vs average).
	volumeScore := volumeSpike(series)

	// Derive regime proxy.
	regime := regimeProxy(series)

	return BehaviorData{
		FragilityScore:     fragilityScore,
		FundingExtremeness: comps.FundingExtremeness,
		OIExpansion:        comps.OIExpansion,
		Imbalance:          comps.LongShortImbalance,
		Regime:             regime,
		VolumeScore:        volumeScore,
		Volatility:         volatility,
	}, nil
}

// fundingProxy estimates funding from price deviation vs SMA.
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
	return (last.Close() - avg) / avg
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

// atrVolatility derives a [0,1] volatility proxy from Average True Range.
func atrVolatility(series domain.CandleSeries, price float64) float64 {
	n := series.Len()
	if n < 2 || price == 0 {
		return 0
	}
	var atrSum float64
	for i := 1; i < n; i++ {
		curr, _ := series.At(i)
		prev, _ := series.At(i - 1)
		tr := math.Max(curr.High()-curr.Low(),
			math.Max(math.Abs(curr.High()-prev.Close()), math.Abs(curr.Low()-prev.Close())))
		atrSum += tr
	}
	atr := atrSum / float64(n-1)
	// Normalize: ATR / price, then scale; typical ATR/price ~ 0.01-0.05
	normalized := (atr / price) * 20 // scale so 5% ATR → 1.0
	if normalized > 1 {
		normalized = 1
	}
	return normalized
}

// volumeSpike returns a [0,1] score indicating how much recent volume exceeds average.
func volumeSpike(series domain.CandleSeries) float64 {
	n := series.Len()
	if n < 2 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		c, _ := series.At(i)
		sum += c.Volume()
	}
	avg := sum / float64(n)
	if avg == 0 {
		return 0
	}
	last, _ := series.At(n - 1)
	ratio := last.Volume() / avg
	// Normalize: ratio of 2x → 1.0
	score := (ratio - 1.0)
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// regimeProxy derives a simple regime label from recent price action.
func regimeProxy(series domain.CandleSeries) string {
	n := series.Len()
	if n < 10 {
		return "unknown"
	}
	// Use last 10 candles: check range compression.
	var high, low float64
	start := n - 10
	c0, _ := series.At(start)
	high = c0.High()
	low = c0.Low()
	for i := start + 1; i < n; i++ {
		c, _ := series.At(i)
		if c.High() > high {
			high = c.High()
		}
		if c.Low() < low {
			low = c.Low()
		}
	}
	if low == 0 {
		return "unknown"
	}
	rangeRatio := (high - low) / low
	if rangeRatio < 0.02 {
		return "compression"
	}
	// Check trend: compare first and last close.
	first, _ := series.At(start)
	last, _ := series.At(n - 1)
	changePct := (last.Close() - first.Close()) / first.Close()
	if changePct > 0.03 {
		return "trending_up"
	}
	if changePct < -0.03 {
		return "trending_down"
	}
	return "range"
}
