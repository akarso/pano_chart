package volatility

import (
	"math"
	"time"
)

// Aggregate processes a sorted slice of 1-minute candles and returns
// intraday volatility buckets. Each bucket represents one minute-of-day
// (0..1439) and contains the average ATR-normalized move, spike
// probability, and a global-average-normalized score.
func Aggregate(candles []Candle) Result {
	buckets := make([]*Bucket, 1440)
	for i := range buckets {
		buckets[i] = &Bucket{MinuteOfDay: i}
	}

	const atrPeriod = 14
	atr := ComputeATR(candles, atrPeriod)

	for i := atrPeriod; i < len(candles); i++ {
		c := candles[i]
		a := atr[i]
		if a == 0 {
			continue
		}

		move := math.Abs(c.Close-c.Open) / a
		t := time.UnixMilli(c.OpenTime).UTC()
		minute := t.Hour()*60 + t.Minute()

		b := buckets[minute]
		b.Count++
		b.SumMove += move
		if move > 1.5 {
			b.SpikeCount++
		}
	}

	// First pass: compute global average of per-bucket averages.
	var globalSum float64
	var globalCount int
	for _, b := range buckets {
		if b.Count == 0 {
			continue
		}
		globalSum += b.SumMove / float64(b.Count)
		globalCount++
	}
	if globalCount == 0 {
		return Result{}
	}
	globalAvg := globalSum / float64(globalCount)

	// Second pass: build results.
	results := make([]BucketResult, 0, 1440)
	for _, b := range buckets {
		if b.Count == 0 {
			continue
		}
		avg := b.SumMove / float64(b.Count)
		results = append(results, BucketResult{
			MinuteOfDay: b.MinuteOfDay,
			AvgMove:     avg,
			SpikeProb:   float64(b.SpikeCount) / float64(b.Count),
			Normalized:  avg / globalAvg,
		})
	}

	return Result{Buckets: results}
}

// ComputeATR returns the exponential ATR value for each candle.
// The first candle has ATR 0 (no previous close).
func ComputeATR(candles []Candle, period int) []float64 {
	n := len(candles)
	atr := make([]float64, n)

	for i := 1; i < n; i++ {
		tr := math.Max(
			candles[i].High-candles[i].Low,
			math.Max(
				math.Abs(candles[i].High-candles[i-1].Close),
				math.Abs(candles[i].Low-candles[i-1].Close),
			),
		)
		if i < period {
			atr[i] = tr
		} else {
			atr[i] = (atr[i-1]*float64(period-1) + tr) / float64(period)
		}
	}

	return atr
}
