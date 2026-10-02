package volatility

// AverageFullResults merges multiple per-symbol profiles into one.
// Equal-weight average of SpikeProb and AvgMove for minutes present in
// *every* input that contributed that series (avoids a gappy altcoin
// inventing sector seasonality on exclusive minutes).
//
// Weekly: only members that already have weekly buckets participate; empty
// weekly on a member is skipped (not treated as a zero curve), so the
// "every member" rule applies among weekly-bearing inputs only.
// Normalized is recomputed from the averaged AvgMove so
// Normalized ≈ AvgMove / mean(AvgMove), matching per-symbol Aggregate.
func AverageFullResults(in []FullResult) FullResult {
	if len(in) == 0 {
		return FullResult{}
	}
	if len(in) == 1 {
		return cloneFullResult(in[0])
	}

	tfOrder, tfBuckets := collectIntraday(in)
	out := FullResult{
		Intraday: make([]TimeframeResult, 0, len(tfOrder)),
	}
	for _, tf := range tfOrder {
		series := tfBuckets[tf]
		buckets := averageIntradayBuckets(series, len(series))
		recomputeIntradayNormalized(buckets)
		out.Intraday = append(out.Intraday, TimeframeResult{
			Timeframe: tf,
			Buckets:   buckets,
		})
	}
	weeklySeries := collectWeekly(in)
	weekly := averageWeeklyBuckets(weeklySeries, len(weeklySeries))
	recomputeWeeklyNormalized(weekly)
	out.Weekly = WeeklyResult{Buckets: weekly}
	return out
}

func cloneFullResult(r FullResult) FullResult {
	out := FullResult{
		Intraday: make([]TimeframeResult, len(r.Intraday)),
		Weekly:   WeeklyResult{Buckets: append([]WeeklyBucket(nil), r.Weekly.Buckets...)},
	}
	for i, tf := range r.Intraday {
		out.Intraday[i] = TimeframeResult{
			Timeframe: tf.Timeframe,
			Buckets:   append([]BucketResult(nil), tf.Buckets...),
		}
	}
	return out
}

func collectIntraday(in []FullResult) ([]Timeframe, map[Timeframe][][]BucketResult) {
	order := make([]Timeframe, 0)
	seen := make(map[Timeframe]struct{})
	byTF := make(map[Timeframe][][]BucketResult)
	for _, r := range in {
		for _, tf := range r.Intraday {
			if _, ok := seen[tf.Timeframe]; !ok {
				seen[tf.Timeframe] = struct{}{}
				order = append(order, tf.Timeframe)
			}
			byTF[tf.Timeframe] = append(byTF[tf.Timeframe], tf.Buckets)
		}
	}
	return order, byTF
}

func collectWeekly(in []FullResult) [][]WeeklyBucket {
	out := make([][]WeeklyBucket, 0, len(in))
	for _, r := range in {
		if len(r.Weekly.Buckets) == 0 {
			continue
		}
		out = append(out, r.Weekly.Buckets)
	}
	return out
}

func averageIntradayBuckets(series [][]BucketResult, requireN int) []BucketResult {
	if requireN <= 0 {
		requireN = len(series)
	}
	type acc struct {
		sumMove, sumSpike float64
		n                 int
	}
	byMinute := make(map[int]*acc)
	order := make([]int, 0)
	for _, buckets := range series {
		for _, b := range buckets {
			a, ok := byMinute[b.MinuteOfDay]
			if !ok {
				a = &acc{}
				byMinute[b.MinuteOfDay] = a
				order = append(order, b.MinuteOfDay)
			}
			a.sumMove += b.AvgMove
			a.sumSpike += b.SpikeProb
			a.n++
		}
	}
	out := make([]BucketResult, 0, len(order))
	for _, m := range order {
		a := byMinute[m]
		if a.n < requireN {
			continue
		}
		n := float64(a.n)
		out = append(out, BucketResult{
			MinuteOfDay: m,
			AvgMove:     a.sumMove / n,
			SpikeProb:   a.sumSpike / n,
		})
	}
	return out
}

func averageWeeklyBuckets(series [][]WeeklyBucket, requireN int) []WeeklyBucket {
	if len(series) == 0 {
		return nil
	}
	if requireN <= 0 {
		requireN = len(series)
	}
	type acc struct {
		sumMove, sumSpike float64
		n                 int
	}
	byMinute := make(map[int]*acc)
	order := make([]int, 0)
	for _, buckets := range series {
		for _, b := range buckets {
			a, ok := byMinute[b.MinuteOfWeek]
			if !ok {
				a = &acc{}
				byMinute[b.MinuteOfWeek] = a
				order = append(order, b.MinuteOfWeek)
			}
			a.sumMove += b.AvgMove
			a.sumSpike += b.SpikeProb
			a.n++
		}
	}
	out := make([]WeeklyBucket, 0, len(order))
	for _, m := range order {
		a := byMinute[m]
		if a.n < requireN {
			continue
		}
		n := float64(a.n)
		out = append(out, WeeklyBucket{
			MinuteOfWeek: m,
			AvgMove:      a.sumMove / n,
			SpikeProb:    a.sumSpike / n,
		})
	}
	return out
}

func recomputeIntradayNormalized(buckets []BucketResult) {
	var sum float64
	var n int
	for _, b := range buckets {
		sum += b.AvgMove
		n++
	}
	if n == 0 {
		return
	}
	global := sum / float64(n)
	if global <= 0 {
		return
	}
	for i := range buckets {
		buckets[i].Normalized = buckets[i].AvgMove / global
	}
}

func recomputeWeeklyNormalized(buckets []WeeklyBucket) {
	var sum float64
	var n int
	for _, b := range buckets {
		sum += b.AvgMove
		n++
	}
	if n == 0 {
		return
	}
	global := sum / float64(n)
	if global <= 0 {
		return
	}
	for i := range buckets {
		buckets[i].Normalized = buckets[i].AvgMove / global
	}
}
