package scoring

import (
	"fmt"
	"math"

	"pano_chart/backend/domain"
)

const trendStrengthWindow = 110

// TrendStrengthScoreCalculator measures persistent direction (ER, swing
// structure, EMA persistence, drawdown, magnitude) instead of OLS R² alone
// (PR-103). Stateless — safe for concurrent use like Trend Predictability.
type TrendStrengthScoreCalculator struct{}

func (c *TrendStrengthScoreCalculator) Name() string {
	return "Trend Predictability"
}

func (c *TrendStrengthScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	score, _, err := c.ScoreWithDirection(series)
	return score, err
}

// ScoreWithDirection implements the PR-103 formula on the last N closes
// (N = min(len, 110)), using log prices and candle highs/lows for swings.
func (c *TrendStrengthScoreCalculator) ScoreWithDirection(series domain.CandleSeries) (score float64, bias string, err error) {
	bias = "neutral"
	n := series.Len()
	if n < 2 {
		return 0, bias, fmt.Errorf("at least 2 candles required")
	}
	start := 0
	if n > trendStrengthWindow {
		start = n - trendStrengthWindow
	}
	m := n - start

	closes := make([]float64, m)
	highs := make([]float64, m)
	lows := make([]float64, m)
	candles := make([]domain.Candle, m)
	for i := 0; i < m; i++ {
		candle, _ := series.At(start + i)
		candles[i] = candle
		closes[i] = candle.Close()
		highs[i] = candle.High()
		lows[i] = candle.Low()
		if closes[i] <= 0 {
			return 0, bias, nil
		}
	}

	p := make([]float64, m)
	for i, c := range closes {
		p[i] = math.Log(c)
	}

	// 1. Efficiency ratio + path concentration
	var path, maxStep float64
	for i := 1; i < m; i++ {
		step := math.Abs(p[i] - p[i-1])
		path += step
		if step > maxStep {
			maxStep = step
		}
	}
	er := 0.0
	if path > 0 {
		er = math.Abs(p[m-1]-p[0]) / path
	}
	er = clamp01(er)
	// Single-bar (or few-bar) jumps are perfectly efficient but not persistent
	// direction — scale ER by how dispersed the path is across bars.
	if path > 0 {
		er *= 1 - clamp01(maxStep/path)
	}
	er = clamp01(er)

	// 2. Swing structure (3-bar pivot window)
	swing := swingStructureScore(highs, lows, er)

	// 3. Persistence via EMA-20 on log prices
	persist := emaPersistenceScore(p, p[m-1]-p[0])

	// 4. Drawdown penalty
	ddPen := drawdownPenalty(p, p[m-1]-p[0])

	// 5. Magnitude gate
	mag := magnitudeGate(candles, p[m-1]-p[0], m)

	structural := 0.4*swing + 0.3*persist + 0.3
	score = math.Pow(er, 0.5) * structural * ddPen * mag
	score = clamp01(score)

	if mag < 0.2 {
		return score, bias, nil
	}
	switch {
	case p[m-1] > p[0]:
		bias = "up"
	case p[m-1] < p[0]:
		bias = "down"
	}
	return score, bias, nil
}

func swingStructureScore(highs, lows []float64, er float64) float64 {
	const pivot = 3
	var swingHighs, swingLows []float64
	for i := pivot; i < len(highs)-pivot; i++ {
		if IsPivotHigh(highs, i, pivot) {
			swingHighs = append(swingHighs, highs[i])
		}
		if IsPivotLow(lows, i, pivot) {
			swingLows = append(swingLows, lows[i])
		}
	}
	if len(swingHighs)+len(swingLows) < 3 {
		return er
	}
	hh := consecutiveHigherFraction(swingHighs)
	hl := consecutiveHigherFraction(swingLows)
	lh := consecutiveLowerFraction(swingHighs)
	ll := consecutiveLowerFraction(swingLows)
	swingUp := (hh + hl) / 2
	swingDown := (lh + ll) / 2
	return clamp01(math.Max(swingUp, swingDown))
}

func consecutiveHigherFraction(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	higher := 0
	for i := 1; i < len(vals); i++ {
		if vals[i] > vals[i-1] {
			higher++
		}
	}
	return float64(higher) / float64(len(vals)-1)
}

func consecutiveLowerFraction(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	lower := 0
	for i := 1; i < len(vals); i++ {
		if vals[i] < vals[i-1] {
			lower++
		}
	}
	return float64(lower) / float64(len(vals)-1)
}

func emaPersistenceScore(p []float64, net float64) float64 {
	const period = 20
	if len(p) < 2 || net == 0 {
		return 0.5
	}
	ema := emaSeries(p, period)
	up := 0
	down := 0
	for i := range p {
		if p[i] > ema[i] {
			up++
		} else if p[i] < ema[i] {
			down++
		}
	}
	if net > 0 {
		return clamp01(float64(up) / float64(len(p)))
	}
	return clamp01(float64(down) / float64(len(p)))
}

func emaSeries(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	alpha := 2.0 / (float64(period) + 1)
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

func drawdownPenalty(p []float64, net float64) float64 {
	minP, maxP := p[0], p[0]
	for _, v := range p {
		if v < minP {
			minP = v
		}
		if v > maxP {
			maxP = v
		}
	}
	span := maxP - minP
	if span <= 0 {
		return 1
	}
	var dd float64
	if net >= 0 {
		dd = (maxP - p[len(p)-1]) / span
	} else {
		dd = (p[len(p)-1] - minP) / span
	}
	return 1 - clamp01(dd*2)
}

func magnitudeGate(candles []domain.Candle, net float64, n int) float64 {
	if n < 2 {
		return 0
	}
	var atrPctSum float64
	count := 0
	for _, c := range candles {
		if c.Close() <= 0 {
			continue
		}
		atrPctSum += (c.High() - c.Low()) / c.Close()
		count++
	}
	if count == 0 {
		return 0
	}
	atrPct := atrPctSum / float64(count)
	if atrPct <= 0 {
		return 0
	}
	denom := atrPct * math.Sqrt(float64(n))
	if denom <= 0 {
		return 0
	}
	return clamp01(math.Abs(net) / denom)
}
