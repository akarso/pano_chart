package volatility_test

import (
	"math"
	"testing"

	vol "pano_chart/backend/infrastructure/volatility"
)

func TestAverageFullResults_AveragesSpikeProb(t *testing.T) {
	a := vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{
				Timeframe: vol.TF1m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 10, AvgMove: 1, SpikeProb: 0.2, Normalized: 0.4},
					{MinuteOfDay: 11, AvgMove: 2, SpikeProb: 0.4, Normalized: 0.8},
				},
			},
			// Coarse TF present in input must be ignored / re-derived from avg 1m.
			{
				Timeframe: vol.TF5m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 0, AvgMove: 99, SpikeProb: 0.99, Normalized: 1},
				},
			},
		},
		Weekly: vol.WeeklyResult{Buckets: []vol.WeeklyBucket{
			{MinuteOfWeek: 100, AvgMove: 1, SpikeProb: 0.1, Normalized: 0.2},
		}},
	}
	b := vol.FullResult{
		Intraday: []vol.TimeframeResult{
			{
				Timeframe: vol.TF1m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 10, AvgMove: 3, SpikeProb: 0.6, Normalized: 0.8},
				},
			},
			{
				Timeframe: vol.TF5m,
				Buckets: []vol.BucketResult{
					{MinuteOfDay: 0, AvgMove: 4, SpikeProb: 0.4, Normalized: 1},
				},
			},
		},
		Weekly: vol.WeeklyResult{Buckets: []vol.WeeklyBucket{
			{MinuteOfWeek: 100, AvgMove: 3, SpikeProb: 0.5, Normalized: 0.6},
		}},
	}
	got := vol.AverageFullResults([]vol.FullResult{a, b})

	byMin := map[int]vol.BucketResult{}
	var has1m, has5m, has1d bool
	for _, tf := range got.Intraday {
		switch tf.Timeframe {
		case vol.TF1m:
			has1m = true
			for _, bk := range tf.Buckets {
				byMin[bk.MinuteOfDay] = bk
			}
		case vol.TF5m:
			has5m = true
			// Must not carry the bogus input 5m spike 0.99.
			for _, bk := range tf.Buckets {
				if bk.SpikeProb > 0.9 {
					t.Fatalf("5m must be re-derived from averaged 1m, got %+v", bk)
				}
			}
		case vol.TF1d:
			has1d = true
		}
	}
	if !has1m || !has5m || !has1d {
		t.Fatalf("expected 1m+derived TFs+1d, got %+v", got.Intraday)
	}
	if _, ok := byMin[11]; ok {
		t.Fatal("exclusive minute 11 must be dropped when not in all members")
	}
	m10 := byMin[10]
	if math.Abs(m10.SpikeProb-0.4) > 1e-9 || math.Abs(m10.AvgMove-2) > 1e-9 {
		t.Fatalf("minute 10=%+v want avg spike 0.4 move 2", m10)
	}
	if math.Abs(m10.Normalized-1.0) > 1e-9 {
		t.Fatalf("normalized=%v want 1.0 after recompute", m10.Normalized)
	}
	if len(got.Weekly.Buckets) != 1 || math.Abs(got.Weekly.Buckets[0].SpikeProb-0.3) > 1e-9 {
		t.Fatalf("weekly=%+v", got.Weekly.Buckets)
	}
}

func TestAverageFullResults_EmptyWeeklyOnSomeMembers(t *testing.T) {
	a := vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: 1, AvgMove: 2, SpikeProb: 0.2}},
		}},
		Weekly: vol.WeeklyResult{Buckets: []vol.WeeklyBucket{
			{MinuteOfWeek: 5, AvgMove: 2, SpikeProb: 0.4},
		}},
	}
	b := vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: 1, AvgMove: 4, SpikeProb: 0.6}},
		}},
		// no weekly
	}
	got := vol.AverageFullResults([]vol.FullResult{a, b})
	if len(got.Weekly.Buckets) != 1 {
		t.Fatalf("weekly from members that have it: %+v", got.Weekly.Buckets)
	}
}

func TestAverageFullResults_Empty(t *testing.T) {
	if len(vol.AverageFullResults(nil).Intraday) != 0 {
		t.Fatal("empty input should yield empty result")
	}
}

func TestAverageFullResults_SingleClone(t *testing.T) {
	in := vol.FullResult{
		Intraday: []vol.TimeframeResult{{
			Timeframe: vol.TF1m,
			Buckets:   []vol.BucketResult{{MinuteOfDay: 1, AvgMove: 2, SpikeProb: 0.5, Normalized: 1}},
		}},
	}
	got := vol.AverageFullResults([]vol.FullResult{in})
	if len(got.Intraday) != 1 || got.Intraday[0].Buckets[0].SpikeProb != 0.5 {
		t.Fatalf("single clone=%+v", got)
	}
	got.Intraday[0].Buckets[0].SpikeProb = 0.9
	if in.Intraday[0].Buckets[0].SpikeProb != 0.5 {
		t.Fatal("clone must not alias input buckets")
	}
}
