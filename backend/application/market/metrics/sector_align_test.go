package metrics

import (
	"math"
	"testing"

	mkt "pano_chart/backend/domain/market"
)

func TestAlignedReturns_ClampsToOverlap(t *testing.T) {
	// Sector has an extra trailing bar the market lacks (candle-cache skew).
	// Clamp to shared stamps → both flat on overlap → RS ≈ 0, available.
	sector := []mkt.IndexPoint{
		{Timestamp: 200, Value: 100},
		{Timestamp: 300, Value: 100},
		{Timestamp: 400, Value: 150}, // market missing; would inflate own return
	}
	market := []mkt.IndexPoint{
		{Timestamp: 100, Value: 100},
		{Timestamp: 200, Value: 110}, // already rallied
		{Timestamp: 300, Value: 110}, // flat on overlap
	}
	win, ok := alignedReturns(sector, market)
	if !ok {
		t.Fatal("expected clamp to succeed")
	}
	if len(win.clamped) != 2 || win.clamped[len(win.clamped)-1].Timestamp != 300 {
		t.Fatalf("clamped=%+v", win.clamped)
	}
	if math.Abs(win.clamped[0].Value-100) > 1e-12 {
		t.Fatalf("want rebased start 100, got %g", win.clamped[0].Value)
	}
	if math.Abs(win.sectorRet) > 1e-12 || math.Abs(win.marketRet) > 1e-12 || math.Abs(win.rs) > 1e-12 {
		t.Fatalf("sectorRet=%g marketRet=%g rs=%g", win.sectorRet, win.marketRet, win.rs)
	}
}

func TestAlignedReturns_StartTrimRebasesTo100(t *testing.T) {
	// Market anchored one bar later than the sector — start-trim would leave
	// points[0]=110 without rebase.
	sector := []mkt.IndexPoint{
		{Timestamp: 100, Value: 100},
		{Timestamp: 200, Value: 110}, // first shared
		{Timestamp: 300, Value: 121},
	}
	market := []mkt.IndexPoint{
		{Timestamp: 200, Value: 100},
		{Timestamp: 300, Value: 100},
	}
	wantRet := math.Log(121.0 / 110.0)
	win, ok := alignedReturns(sector, market)
	if !ok {
		t.Fatal("expected clamp to succeed")
	}
	if len(win.clamped) != 2 || win.clamped[0].Timestamp != 200 {
		t.Fatalf("clamped=%+v", win.clamped)
	}
	if math.Abs(win.clamped[0].Value-100) > 1e-12 {
		t.Fatalf("want points[0]=100 after rebase, got %g", win.clamped[0].Value)
	}
	if math.Abs(win.clamped[1].Value-110) > 1e-9 {
		t.Fatalf("want rebased last=110, got %g", win.clamped[1].Value)
	}
	if math.Abs(win.sectorRet-wantRet) > 1e-12 {
		t.Fatalf("rebase must not change return: got %g want %g", win.sectorRet, wantRet)
	}
	if math.Abs(win.rs-wantRet) > 1e-12 { // market flat on overlap
		t.Fatalf("rs=%g want %g", win.rs, wantRet)
	}
}

func TestAlignedReturns_FewerThanTwoCommonUnavailable(t *testing.T) {
	sector := []mkt.IndexPoint{
		{Timestamp: 100, Value: 100},
		{Timestamp: 200, Value: 110},
	}
	market := []mkt.IndexPoint{
		{Timestamp: 100, Value: 100},
		{Timestamp: 150, Value: 105}, // no shared second stamp
	}
	win, ok := alignedReturns(sector, market)
	if ok {
		t.Fatal("expected unavailable")
	}
	if win.clamped != nil || win.rs != 0 {
		t.Fatalf("win=%+v", win)
	}
}

func TestMatchingPathPoints_BothMedianWhenSectorLacksVW(t *testing.T) {
	sector := CompositeTape{
		Index: mkt.CompositeIndex{
			Points: []mkt.IndexPoint{{Timestamp: 1, Value: 100}, {Timestamp: 2, Value: 101}},
		},
	}
	market := CompositeTape{
		Index: mkt.CompositeIndex{
			Points:               []mkt.IndexPoint{{Timestamp: 1, Value: 100}, {Timestamp: 2, Value: 102}},
			VolumeWeightedPoints: []mkt.IndexPoint{{Timestamp: 1, Value: 100}, {Timestamp: 2, Value: 103}},
		},
		PreferredSource: "composite_volume_weighted",
	}
	sp, mp, ok := matchingPathPoints(sector, market)
	if !ok || sp[1].Value != 101 || mp[1].Value != 102 {
		t.Fatalf("sp=%v mp=%v ok=%v", sp, mp, ok)
	}
}

func TestPointsForPath_NoSilentFallback(t *testing.T) {
	tape := CompositeTape{
		Index: mkt.CompositeIndex{
			Points: []mkt.IndexPoint{{Timestamp: 1, Value: 100}, {Timestamp: 2, Value: 101}},
		},
	}
	if _, ok := pointsForPath(tape, "composite_volume_weighted"); ok {
		t.Fatal("VW should miss")
	}
}

func TestSortByRS_UnavailableLast(t *testing.T) {
	sectors := []mkt.SectorIndex{
		{ID: "z", RS: 0, RSAvailable: false},
		{ID: "a", RS: 0.1, RSAvailable: true},
		{ID: "b", RS: 0.2, RSAvailable: true},
	}
	sortByRS(sectors)
	if sectors[0].ID != "b" || sectors[1].ID != "a" || sectors[2].ID != "z" {
		t.Fatalf("%+v", sectors)
	}
}
