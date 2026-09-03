package volatility

// DeriveTimeframe aggregates 1-minute BucketResults into coarser
// timeframe buckets by averaging groups of `groupSize` consecutive
// entries. No ATR recomputation — the already-normalized values are
// averaged directly.
func DeriveTimeframe(base []BucketResult, groupSize int, tf Timeframe) TimeframeResult {
	if groupSize <= 0 || len(base) == 0 {
		return TimeframeResult{Timeframe: tf}
	}

	result := make([]BucketResult, 0, (len(base)+groupSize-1)/groupSize)

	for i := 0; i < len(base); i += groupSize {
		var sumMove, sumSpike, sumNorm float64
		count := 0

		for j := 0; j < groupSize && i+j < len(base); j++ {
			b := base[i+j]
			sumMove += b.AvgMove
			sumSpike += b.SpikeProb
			sumNorm += b.Normalized
			count++
		}

		result = append(result, BucketResult{
			MinuteOfDay: base[i].MinuteOfDay,
			AvgMove:     sumMove / float64(count),
			SpikeProb:   sumSpike / float64(count),
			Normalized:  sumNorm / float64(count),
		})
	}

	return TimeframeResult{Timeframe: tf, Buckets: result}
}

// BuildAllTimeframes derives all standard timeframes from 1-minute base
// buckets.  The 1d timeframe is intentionally excluded here — it is
// derived from weekly day-of-week data instead (see DeriveDailyOfWeek).
func BuildAllTimeframes(base []BucketResult) []TimeframeResult {
	return []TimeframeResult{
		{Timeframe: TF1m, Buckets: base},
		DeriveTimeframe(base, 5, TF5m),
		DeriveTimeframe(base, 15, TF15m),
		DeriveTimeframe(base, 60, TF1h),
		DeriveTimeframe(base, 240, TF4h),
	}
}
