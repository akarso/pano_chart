package volatility

import (
	"math"
	"sort"
	"time"
)

// weeklyAccumulator collects per-minute-of-week statistics.
type weeklyAccumulator struct {
	count      int
	sumMove    float64
	spikeCount float64
}

// BuildWeekly computes 7-day (10 080 minute) seasonality from raw
// 1-minute candles and their pre-computed ATR values.
// The candles and atr slices must be the same length.
func BuildWeekly(candles []Candle, atr []float64) WeeklyResult {
	if len(candles) == 0 || len(atr) == 0 {
		return WeeklyResult{}
	}

	buckets := make(map[int]*weeklyAccumulator)

	for i := range candles {
		a := atr[i]
		if a == 0 {
			continue
		}

		t := time.UnixMilli(candles[i].OpenTime).UTC()
		dow := int(t.Weekday()) // 0 = Sunday
		minute := t.Hour()*60 + t.Minute()
		idx := dow*1440 + minute

		acc, ok := buckets[idx]
		if !ok {
			acc = &weeklyAccumulator{}
			buckets[idx] = acc
		}

		move := math.Abs(candles[i].Close-candles[i].Open) / a
		acc.count++
		acc.sumMove += move
		if move > 1.5 {
			acc.spikeCount++
		}
	}

	if len(buckets) == 0 {
		return WeeklyResult{}
	}

	// Global average for normalization.
	var total float64
	var count int
	for _, b := range buckets {
		total += b.sumMove / float64(b.count)
		count++
	}
	globalAvg := total / float64(count)

	results := make([]WeeklyBucket, 0, len(buckets))
	for k, b := range buckets {
		avg := b.sumMove / float64(b.count)
		results = append(results, WeeklyBucket{
			MinuteOfWeek: k,
			AvgMove:      avg,
			SpikeProb:    b.spikeCount / float64(b.count),
			Normalized:   avg / globalAvg,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].MinuteOfWeek < results[j].MinuteOfWeek
	})

	return WeeklyResult{Buckets: results}
}
