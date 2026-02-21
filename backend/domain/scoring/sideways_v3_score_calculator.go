package scoring

import (
	"fmt"
	"math"
	"sort"

	"pano_chart/backend/domain"
)

// SidewaysV3Config holds timeframe-dependent thresholds for the channel
// range preference score.
type SidewaysV3Config struct {
	RMin float64 // minimum relative channel height
	RMax float64 // maximum relative channel height
}

// DefaultSidewaysV3Config returns reasonable defaults for common timeframes.
func DefaultSidewaysV3Config(tf string) SidewaysV3Config {
	switch tf {
	case "1m":
		return SidewaysV3Config{RMin: 0.002, RMax: 0.02}
	case "5m":
		return SidewaysV3Config{RMin: 0.003, RMax: 0.03}
	case "15m":
		return SidewaysV3Config{RMin: 0.005, RMax: 0.05}
	case "1h":
		return SidewaysV3Config{RMin: 0.008, RMax: 0.06}
	case "4h":
		return SidewaysV3Config{RMin: 0.01, RMax: 0.08}
	case "1d":
		return SidewaysV3Config{RMin: 0.02, RMax: 0.12}
	default:
		return SidewaysV3Config{RMin: 0.008, RMax: 0.06} // 1h default
	}
}

// SidewaysV3ScoreCalculator scores channel structure via robust envelope,
// containment, medium-range preference, boundary balance, and drift penalty.
// All factors are multiplicative: one structural failure collapses the score.
type SidewaysV3ScoreCalculator struct {
	Config SidewaysV3Config
}

func (c *SidewaysV3ScoreCalculator) Name() string {
	return "Sideways Consistency"
}

func (c *SidewaysV3ScoreCalculator) Score(series domain.CandleSeries) (float64, error) {
	n := series.Len()
	if n < 20 {
		return 0, fmt.Errorf("at least 20 candles required, got %d", n)
	}

	// Step 1: Robust envelope from highs/lows.
	upper, lower := computeTrimmedEnvelope(series)
	H := upper - lower
	if H <= 0 {
		return 0, nil
	}

	// Step 2: Containment.
	closes := extractCloses(series)
	containment := computeContainment(closes, lower, upper)
	if containment < 0.75 {
		return 0, nil
	}

	// Step 3: Medium range preference.
	rangeScore := computeRangeScore(closes, H, c.Config)
	if rangeScore == 0 {
		return 0, nil
	}

	// Step 4: Boundary balance.
	balanceScore := computeBalanceScore(closes, lower, upper, H)
	if balanceScore == 0 {
		return 0, nil
	}

	// Step 5: Drift penalty.
	driftPenalty := computeDriftPenalty(closes, H)
	if driftPenalty == 0 {
		return 0, nil
	}

	score := containment * rangeScore * balanceScore * driftPenalty
	return clamp01(score), nil
}

// computeTrimmedEnvelope returns the 95th-percentile of highs and the
// 5th-percentile of lows, making the envelope resistant to single outlier
// spikes.
func computeTrimmedEnvelope(series domain.CandleSeries) (upper, lower float64) {
	n := series.Len()
	highs := make([]float64, n)
	lows := make([]float64, n)
	for i := 0; i < n; i++ {
		candle, _ := series.At(i)
		highs[i] = candle.High()
		lows[i] = candle.Low()
	}
	sort.Float64s(highs)
	sort.Float64s(lows)
	upper = percentile(highs, 0.95)
	lower = percentile(lows, 0.05)
	return
}

// computeContainment returns the fraction of closes inside [lower, upper].
func computeContainment(closes []float64, lower, upper float64) float64 {
	inside := 0
	for _, c := range closes {
		if c >= lower && c <= upper {
			inside++
		}
	}
	return float64(inside) / float64(len(closes))
}

// computeRangeScore prefers medium channel height relative to mean price.
// Returns 0 for channels that are too tight or too wide.
func computeRangeScore(closes []float64, H float64, cfg SidewaysV3Config) float64 {
	_, _, meanPrice := priceStats(closes)
	if meanPrice == 0 {
		return 0
	}
	R := H / meanPrice
	if R <= cfg.RMin || R >= cfg.RMax {
		return 0
	}
	mid := (cfg.RMin + cfg.RMax) / 2
	span := cfg.RMax - cfg.RMin
	score := 1 - math.Abs(R-mid)/(span/2)
	return clamp01(score)
}

// computeBalanceScore checks that the price touches both upper and lower
// channel edges.  Returns 0 if either side has zero touches.
func computeBalanceScore(closes []float64, lower, upper, H float64) float64 {
	epsilon := 0.05 * H
	upperTouches := 0
	lowerTouches := 0
	for _, c := range closes {
		if c >= upper-epsilon {
			upperTouches++
		}
		if c <= lower+epsilon {
			lowerTouches++
		}
	}
	if upperTouches == 0 || lowerTouches == 0 {
		return 0
	}
	totalTouches := upperTouches + lowerTouches
	imbalance := math.Abs(float64(upperTouches-lowerTouches)) / float64(totalTouches)
	return clamp01(1 - imbalance)
}

// computeDriftPenalty penalises net displacement relative to channel height.
func computeDriftPenalty(closes []float64, H float64) float64 {
	if len(closes) == 0 || H == 0 {
		return 0
	}
	netDrift := math.Abs(closes[len(closes)-1] - closes[0])
	driftRatio := netDrift / H
	if driftRatio >= 1 {
		return 0
	}
	return clamp01(1 - driftRatio)
}
