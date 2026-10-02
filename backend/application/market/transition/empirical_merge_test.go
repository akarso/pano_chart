package transition

import (
	"testing"

	mkt "pano_chart/backend/domain/market"
)

func TestNormalizeAndMerge_ClosedStubPreservedWhenMergedIntoOpen(t *testing.T) {
	end := int64(1)
	segs := normalizeAndMerge([]mkt.RegimePeriod{
		{Regime: mkt.RegimeSilent, DurationCandles: 50, EndTimestamp: &end},
		{Regime: mkt.RegimeIndecisive, DurationCandles: 10_000, EndTimestamp: nil},
	})
	if len(segs) != 1 || len(segs[0].periods) != 1 {
		t.Fatalf("segs=%+v", segs)
	}
	if segs[0].periods[0].EndTimestamp != nil {
		t.Fatal("merged row should be open")
	}
	stubs := segs[0].medianClosed[mkt.RegimeSideways]
	if len(stubs) != 1 || stubs[0] != 50 {
		t.Fatalf("medianClosed stubs=%v want [50]", stubs)
	}
}
