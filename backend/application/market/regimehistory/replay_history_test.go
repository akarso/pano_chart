package regimehistory

import (
	"testing"

	mkt "pano_chart/backend/domain/market"
)

func TestHistoryAtAsOf_IncludesCoveringPeriod(t *testing.T) {
	end1 := int64(1_700_000_000)
	end2 := int64(1_700_100_000)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeTrend, StartTimestamp: 0, EndTimestamp: &end1, DurationCandles: 3},
		{Regime: mkt.RegimeSideways, StartTimestamp: end1, EndTimestamp: &end2, DurationCandles: 5},
		{Regime: mkt.RegimeCompression, StartTimestamp: end2, EndTimestamp: nil, DurationCandles: 99},
	}
	// asOf inside the sideways period (before compression started).
	asOf := end1 + 1000
	out, age := historyAtAsOf(periods, "4h", asOf, 50)
	if len(out) != 2 {
		t.Fatalf("periods=%d want trend + truncated sideways", len(out))
	}
	if out[1].Regime != mkt.RegimeSideways || out[1].EndTimestamp != nil {
		t.Fatalf("second=%+v want open sideways at asOf", out[1])
	}
	if age != out[1].DurationCandles || age <= 0 {
		t.Fatalf("age=%d duration=%d", age, out[1].DurationCandles)
	}
}

func TestHistoryAtAsOf_GapAgeIsZero(t *testing.T) {
	end1 := int64(100)
	end2 := int64(200)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeTrend, StartTimestamp: 0, EndTimestamp: &end1, DurationCandles: 3},
		{Regime: mkt.RegimeSideways, StartTimestamp: end1, EndTimestamp: &end2, DurationCandles: 5},
		// Gap: next period starts at 300 — asOf=250 has no covering period.
		{Regime: mkt.RegimeCompression, StartTimestamp: 300, EndTimestamp: nil, DurationCandles: 2},
	}
	out, age := historyAtAsOf(periods, "4h", 250, 50)
	if age != 0 {
		t.Fatalf("gap age=%d want 0 (not last closed duration)", age)
	}
	if len(out) != 2 {
		t.Fatalf("periods=%d want closed predecessors only", len(out))
	}
}

func TestAgeCandlesAt_CountsBoundaries(t *testing.T) {
	start := int64(1_700_000_000)
	asOf := start + 3*4*3600 // three 4h steps
	got := ageCandlesAt("4h", start, asOf)
	if got != 4 {
		t.Fatalf("age=%d want 4", got)
	}
}
