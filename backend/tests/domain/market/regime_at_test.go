package market_test

import (
	"testing"

	mkt "pano_chart/backend/domain/market"
)

func TestRegimeAt_halfOpen(t *testing.T) {
	end1 := int64(100)
	end2 := int64(200)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, StartTimestamp: 0, EndTimestamp: &end1},
		{Regime: mkt.RegimeTrend, StartTimestamp: 100, EndTimestamp: &end2}, // shared boundary
		{Regime: mkt.RegimeSideways, StartTimestamp: 200, EndTimestamp: nil},
	}
	cases := []struct {
		at   int64
		want mkt.Regime
		ok   bool
	}{
		{50, mkt.RegimeCompression, true},
		{99, mkt.RegimeCompression, true},
		{100, mkt.RegimeTrend, true}, // boundary belongs to new regime
		{150, mkt.RegimeTrend, true},
		{200, mkt.RegimeSideways, true},
		{250, mkt.RegimeSideways, true},
		{-1, "", false},
	}
	for _, tc := range cases {
		got, ok := mkt.RegimeAt(periods, tc.at)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("at=%d: got (%q,%v) want (%q,%v)", tc.at, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRegimeAt_trackerSharedBoundary(t *testing.T) {
	// Production: CloseCurrent(ts) then Append(Start=ts) — same unix instant.
	boundary := int64(1000)
	periods := []mkt.RegimePeriod{
		{Regime: mkt.RegimeCompression, StartTimestamp: 0, EndTimestamp: &boundary},
		{Regime: mkt.RegimeExpansion, StartTimestamp: boundary, EndTimestamp: nil},
	}
	got, ok := mkt.RegimeAt(periods, boundary)
	if !ok || got != mkt.RegimeExpansion {
		t.Fatalf("got (%q,%v) want expansion at shared boundary", got, ok)
	}
}
